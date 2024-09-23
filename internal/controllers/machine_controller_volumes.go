// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controllers

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/digitalocean/go-libvirt"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/ironcore-dev/libvirt-provider/api"
	providerhost "github.com/ironcore-dev/libvirt-provider/internal/host"
	libvirtutils "github.com/ironcore-dev/libvirt-provider/internal/libvirt/utils"
	providervolume "github.com/ironcore-dev/libvirt-provider/internal/plugins/volume"
	"k8s.io/apimachinery/pkg/util/sets"
	utilstrings "k8s.io/utils/strings"
	"libvirt.org/go/libvirtxml"
)

func (r *MachineReconciler) deleteVolumes(ctx context.Context, log logr.Logger, machine *api.Machine) error {
	mounter := r.machineVolumeMounter(machine)
	var errs []error

	if err := mounter.ForEachVolume(func(volume *MountVolume) bool {
		log.V(1).Info("Unmounting volume", "volumeName", volume.ComputeVolumeName)
		if err := mounter.DeleteVolume(ctx, volume.ComputeVolumeName); err != nil {
			errs = append(errs, fmt.Errorf("[volume %s] error deleting volume: %w", volume.ComputeVolumeName, err))
		}
		return true
	}); err != nil && !errors.Is(err, os.ErrNotExist) {
		if len(errs) > 0 {
			log.Error(fmt.Errorf("%v", errs), "Error(s) deleting volumes")
		}
		return fmt.Errorf("error iterating mounted volumes: %w", err)
	}

	if len(errs) > 0 {
		return fmt.Errorf("error(s) deleting volumes: %v", errs)
	}

	log.V(1).Info("All volumes cleaned up, removing volumes directory")
	if err := os.RemoveAll(r.host.MachineVolumesDir(machine.ID)); err != nil {
		return fmt.Errorf("error removing machine volumes directory: %w", err)
	}
	return nil
}

func GetUniqueVolumeName(pluginName, backingVolumeID string) string {
	return fmt.Sprintf("%s/%s", pluginName, backingVolumeID)
}

func (r *MachineReconciler) machineVolumeMounter(machine *api.Machine) VolumeMounter {
	return &volumeMounter{
		host:          r.host,
		pluginManager: r.volumePluginManager,
		machine:       machine,
	}
}

func getVolumeStatus(machine *api.Machine, volumeID string) *api.VolumeStatus {
	for _, volumeStatus := range machine.Status.VolumeStatus {
		if volumeID == volumeStatus.Handle {
			return &volumeStatus
		}
	}
	return nil
}

func getLastVolumeSize(machine *api.Machine, volumeID string) int64 {
	if status := getVolumeStatus(machine, volumeID); status != nil && status.Size != 0 {
		return status.Size
	}
	return 0
}

func (r *MachineReconciler) attachDetachVolumes(ctx context.Context, log logr.Logger, machine *api.Machine, attacher VolumeAttacher) ([]api.VolumeStatus, error) {
	mounter := r.machineVolumeMounter(machine)
	specVolumes := r.listDesiredVolumes(machine)

	currentVolumeNames := sets.New[string]()
	if err := attacher.ForEachVolume(func(volume *AttachVolume) bool {
		currentVolumeNames.Insert(volume.Name)
		return true
	}); err != nil {
		return nil, fmt.Errorf("error iterating attached volumes: %w", err)
	}

	if err := mounter.ForEachVolume(func(volume *MountVolume) bool {
		currentVolumeNames.Insert(volume.ComputeVolumeName)
		return true
	}); err != nil {
		return nil, fmt.Errorf("error iterating mounted volumes: %w", err)
	}

	var errs []error
	for volumeName := range currentVolumeNames {
		if _, ok := specVolumes[volumeName]; ok {
			continue
		}

		log.V(1).Info("Deleting non-required volume", "volumeName", volumeName)
		if err := r.deleteVolume(ctx, log, mounter, attacher, volumeName); err != nil {
			errs = append(errs, fmt.Errorf("[volume %s] error detaching: %w", volumeName, err))
		} else {
			log.V(1).Info("Successfully detached volume", "volumeName", volumeName)
		}
	}

	var volumeStates []api.VolumeStatus
	for _, volume := range specVolumes {
		log.V(1).Info("Reconciling volume", "volumeName", volume.Name)
		volumeID, volumeSize, err := r.applyVolume(ctx, log, machine, volume, mounter, attacher)
		if err != nil {
			errs = append(errs, fmt.Errorf("[volume %s] error reconciling: %w", volume.Name, err))
			continue
		}

		log.V(1).Info("Successfully reconciled volume", "volumeName", volume.Name, "volumeID", volumeID)
		volumeStates = append(volumeStates, api.VolumeStatus{
			Name:   volume.Name,
			Handle: volumeID,
			State:  api.VolumeStateAttached,
			Size:   volumeSize,
		})
	}

	if len(errs) > 0 {
		return nil, fmt.Errorf("attach/detach error(s): %v", errs)
	}
	return volumeStates, nil
}

func (r *MachineReconciler) deleteVolume(ctx context.Context, log logr.Logger, mounter VolumeMounter, attacher VolumeAttacher, volumeName string) error {
	log.V(1).Info("Detaching volume if attached")
	if err := attacher.DetachVolume(volumeName); err != nil && !errors.Is(err, ErrAttachedVolumeNotFound) {
		return fmt.Errorf("error detaching volume: %w", err)
	}

	log.V(1).Info("Unmounting volume if mounted")
	if err := mounter.DeleteVolume(ctx, volumeName); err != nil && !errors.Is(err, ErrMountedVolumeNotFound) {
		return fmt.Errorf("error unmounting volume: %w", err)
	}

	return nil
}

type AttachVolume struct {
	Name   string
	Device string
	Spec   providervolume.Volume
}

type VolumeAttacher interface {
	ListVolumes() ([]AttachVolume, error)
	ForEachVolume(f func(*AttachVolume) bool) error
	GetVolume(name string) (*AttachVolume, error)
	AttachVolume(volume *AttachVolume) error
	DetachVolume(name string) error
	ResizeVolume(volume *AttachVolume) error
}

var (
	ErrAttachedVolumeNotFound      = errors.New("volume not found")
	ErrAttachedVolumeAlreadyExists = errors.New("volume already exists")
)

type DomainExecutor interface {
	AttachDisk(disk *libvirtxml.DomainDisk) error
	DetachDisk(disk *libvirtxml.DomainDisk) error
	ResizeDisk(device string, size int64) error

	ApplySecret(secret *libvirtxml.Secret, data []byte) error
	DeleteSecret(secretUUID string) error
}

type createDomainExecutor struct {
	libvirt *libvirt.Libvirt
}

func NewCreateDomainExecutor(lv *libvirt.Libvirt) DomainExecutor {
	return &createDomainExecutor{libvirt: lv}
}

func (e *createDomainExecutor) AttachDisk(*libvirtxml.DomainDisk) error { return nil }
func (e *createDomainExecutor) DetachDisk(*libvirtxml.DomainDisk) error { return nil }
func (e *createDomainExecutor) ResizeDisk(string, int64) error          { return nil }
func (e *createDomainExecutor) ApplySecret(secret *libvirtxml.Secret, value []byte) error {
	return libvirtutils.ApplySecret(e.libvirt, secret, value)
}

func (e *createDomainExecutor) DeleteSecret(secretUUID string) error {
	return e.libvirt.SecretUndefine(libvirt.Secret{
		UUID: libvirtutils.UUIDStringToBytes(secretUUID),
	})
}

type domainExecutor struct {
	libvirt   *libvirt.Libvirt
	machineID string
}

func NewRunningDomainExecutor(lv *libvirt.Libvirt, machineID string) DomainExecutor {
	return &domainExecutor{
		libvirt:   lv,
		machineID: machineID,
	}
}

func (a *domainExecutor) domain() libvirt.Domain {
	return machineDomain(a.machineID)
}

func (a *domainExecutor) AttachDisk(disk *libvirtxml.DomainDisk) error {
	data, err := disk.Marshal()
	if err != nil {
		return err
	}

	return a.libvirt.DomainAttachDevice(a.domain(), data)
}

func (a *domainExecutor) DetachDisk(disk *libvirtxml.DomainDisk) error {
	data, err := disk.Marshal()
	if err != nil {
		return err
	}

	return a.libvirt.DomainDetachDevice(a.domain(), data)
}

func (a *domainExecutor) ApplySecret(secret *libvirtxml.Secret, value []byte) error {
	return libvirtutils.ApplySecret(a.libvirt, secret, value)
}

func (a *domainExecutor) DeleteSecret(secretUUID string) error {
	return a.libvirt.SecretUndefine(libvirt.Secret{
		UUID: libvirtutils.UUIDStringToBytes(secretUUID),
	})
}

func (a *domainExecutor) ResizeDisk(device string, size int64) error {
	return a.libvirt.DomainBlockResize(a.domain(), computeVirtioDiskTargetDeviceName(device), uint64(size), libvirt.DomainBlockResizeBytes)
}

type libvirtVolumeAttacher struct {
	domainDesc        *libvirtxml.Domain
	executor          DomainExecutor
	volumeCachePolicy string
}

func NewLibvirtVolumeAttacher(domainDesc *libvirtxml.Domain, executor DomainExecutor, policy string) (VolumeAttacher, error) {
	a := &libvirtVolumeAttacher{
		domainDesc:        domainDesc,
		executor:          executor,
		volumeCachePolicy: policy,
	}
	return a, nil
}

func (a *libvirtVolumeAttacher) domainDevices() *libvirtxml.DomainDeviceList {
	if a.domainDesc.Devices == nil {
		a.domainDesc.Devices = &libvirtxml.DomainDeviceList{}
	}
	return a.domainDesc.Devices
}

func (a *libvirtVolumeAttacher) diskByVolumeNameIndex(name string) (int, error) {
	for i, disk := range a.domainDevices().Disks {
		alias := disk.Alias
		if alias == nil || !isDiskAlias(alias.Name) {
			continue
		}

		parsed, err := parseVolumeDiskAlias(alias.Name)
		if err != nil {
			return 0, err
		}

		if parsed == name {
			return i, nil
		}
	}
	return -1, nil
}

func (a *libvirtVolumeAttacher) ListVolumes() ([]AttachVolume, error) {
	var res []AttachVolume
	if err := a.ForEachVolume(func(volume *AttachVolume) bool {
		res = append(res, *volume)
		return true
	}); err != nil {
		return nil, err
	}
	return res, nil
}

// getDiskTargetDevice returns the deviceName of the disk from the domain xml.
func getDiskTargetDevice(disk *libvirtxml.DomainDisk) (string, error) {
	target := disk.Target
	if target == nil {
		return "", fmt.Errorf("disk has no target")
	}

	device := target.Dev
	if device == "" {
		return "", fmt.Errorf("target has no device")
	}

	return device, nil
}

// computeVirtioDiskTargetDeviceName computes the deviceName for the Virtio volumes from the Machine.Volumes.Device.
func computeVirtioDiskTargetDeviceName(device string) string {
	return "v" + device[1:]
}

func (a *libvirtVolumeAttacher) forEachVolumeAndDisk(f func(*libvirtxml.DomainDisk, *AttachVolume) bool) error {
	for _, disk := range a.domainDevices().Disks {
		alias := disk.Alias
		if alias == nil || !isDiskAlias(alias.Name) {
			continue
		}

		// TODO: Revisit how to handle errors in these cases.
		parsed, err := parseVolumeDiskAlias(alias.Name)
		if err != nil {
			return err
		}

		volume, err := libvirtDiskToProviderVolume(&disk)
		if err != nil {
			return err
		}

		device, err := getDiskTargetDevice(&disk)
		if err != nil {
			return err
		}

		attachedVolume := AttachVolume{
			Name:   parsed,
			Device: device,
			Spec:   *volume,
		}
		if !f(&disk, &attachedVolume) {
			return nil
		}
	}
	return nil
}

func (a *libvirtVolumeAttacher) ForEachVolume(f func(*AttachVolume) bool) error {
	return a.forEachVolumeAndDisk(func(disk *libvirtxml.DomainDisk, volume *AttachVolume) bool {
		return f(volume)
	})
}

func (a *libvirtVolumeAttacher) AttachVolume(volume *AttachVolume) error {
	existingIdx, err := a.diskByVolumeNameIndex(volume.Name)
	if err != nil {
		return err
	}

	if existingIdx != -1 {
		return ErrAttachedVolumeAlreadyExists
	}

	if err := func() error {
		disk, secret, encryptionSecret, secretValue, encryptionSecretValue, err := a.providerVolumeToLibvirt(volume.Name, &volume.Spec, volume.Device)
		if err != nil {
			return err
		}

		if secret != nil {
			if err := a.executor.ApplySecret(secret, secretValue); err != nil {
				return err
			}
		} else {
			if err := a.executor.DeleteSecret(a.secretUUID(volume.Name)); libvirtutils.IgnoreErrorCode(err, libvirt.ErrNoSecret) != nil {
				return err
			}
		}

		if encryptionSecret != nil {
			if err := a.executor.ApplySecret(encryptionSecret, encryptionSecretValue); err != nil {
				return err
			}
		} else {
			if err := a.executor.DeleteSecret(a.secretEncryptionUUID(volume.Name)); libvirtutils.IgnoreErrorCode(err, libvirt.ErrNoSecret) != nil {
				return err
			}
		}

		if err := a.executor.AttachDisk(disk); err != nil {
			return err
		}

		a.domainDevices().Disks = append(a.domainDevices().Disks, *disk)
		return nil
	}(); err != nil {
		return err
	}
	return nil
}

func (a *libvirtVolumeAttacher) DetachVolume(name string) error {
	idx, err := a.diskByVolumeNameIndex(name)
	if err != nil {
		return err
	}
	if idx == -1 {
		return ErrAttachedVolumeNotFound
	}

	disk := &a.domainDevices().Disks[idx]

	if err := a.executor.DetachDisk(disk); err != nil {
		return err
	}

	if err := a.executor.DeleteSecret(a.secretUUID(name)); libvirtutils.IgnoreErrorCode(err, libvirt.ErrNoSecret) != nil {
		return err
	}

	if err := a.executor.DeleteSecret(a.secretEncryptionUUID(name)); libvirtutils.IgnoreErrorCode(err, libvirt.ErrNoSecret) != nil {
		return err
	}

	a.domainDevices().Disks = slices.Delete(a.domainDevices().Disks, idx, idx)

	return nil
}

func (a *libvirtVolumeAttacher) ResizeVolume(volume *AttachVolume) error {
	return a.executor.ResizeDisk(volume.Device, volume.Spec.Size)
}

func (a *libvirtVolumeAttacher) GetVolume(name string) (*AttachVolume, error) {
	idx, err := a.diskByVolumeNameIndex(name)
	if err != nil {
		return nil, err
	}
	if idx == -1 {
		return nil, ErrAttachedVolumeNotFound
	}

	disk := &a.domainDevices().Disks[idx]

	volume, err := libvirtDiskToProviderVolume(disk)
	if err != nil {
		return nil, err
	}

	device, err := getDiskTargetDevice(disk)
	if err != nil {
		return nil, err
	}

	return &AttachVolume{
		Name:   name,
		Device: device,
		Spec:   *volume,
	}, nil
}

type MountVolume = providerhost.MachineVolume

type volumeMounter struct {
	host          providerhost.Host
	pluginManager *providervolume.PluginManager
	machine       *api.Machine
}

type VolumeMounter interface {
	PluginManager() *providervolume.PluginManager

	ForEachVolume(f func(*MountVolume) bool) error
	ListVolumes() ([]MountVolume, error)
	GetVolume(computeVolumeName string) (*MountVolume, error)
	ApplyVolume(ctx context.Context, spec *api.VolumeSpec, onDelete func(*MountVolume) error) (string, *providervolume.Volume, error)
	DeleteVolume(ctx context.Context, computeVolumeName string) error
}

var (
	ErrMountedVolumeNotFound = errors.New("mounted volume not found")
)

func (m *volumeMounter) PluginManager() *providervolume.PluginManager {
	return m.pluginManager
}

func (m *volumeMounter) ForEachVolume(f func(*providerhost.MachineVolume) bool) error {
	machineVolumesDir := m.host.MachineVolumesDir(m.machine.ID)
	volumeDirEntries, err := os.ReadDir(machineVolumesDir)
	if err != nil {
		return err
	}

	for _, volumeDirEntry := range volumeDirEntries {
		if !volumeDirEntry.IsDir() {
			continue
		}

		pluginName := utilstrings.UnescapeQualifiedName(volumeDirEntry.Name())
		pluginEntries, err := os.ReadDir(filepath.Join(machineVolumesDir, volumeDirEntry.Name()))
		if err != nil {
			return err
		}

		for _, pluginEntry := range pluginEntries {
			if !pluginEntry.IsDir() {
				continue
			}

			mountedVolume := &MountVolume{
				PluginName:        pluginName,
				ComputeVolumeName: pluginEntry.Name(),
			}
			if !f(mountedVolume) {
				return nil
			}
		}
	}
	return nil
}

func (m *volumeMounter) GetVolume(computeVolumeName string) (*MountVolume, error) {
	var found *MountVolume
	if err := m.ForEachVolume(func(volume *providerhost.MachineVolume) bool {
		if volume.ComputeVolumeName == computeVolumeName {
			res := *volume
			found = &res
			return false
		}
		return true
	}); err != nil {
		return nil, err
	}
	if found == nil {
		return nil, ErrMountedVolumeNotFound
	}
	return found, nil
}

func (m *volumeMounter) ListVolumes() ([]MountVolume, error) {
	var res []MountVolume
	if err := m.ForEachVolume(func(volume *providerhost.MachineVolume) bool {
		res = append(res, *volume)
		return true
	}); err != nil {
		return nil, err
	}
	return res, nil
}

func (m *volumeMounter) DeleteVolume(ctx context.Context, computeVolumeName string) error {
	mountedVolume, err := m.GetVolume(computeVolumeName)
	if err != nil {
		return err
	}

	plugin, err := m.pluginManager.FindPluginByName(mountedVolume.PluginName)
	if err != nil {
		return err
	}

	if err := plugin.Delete(ctx, computeVolumeName, m.machine.ID); err != nil {
		return err
	}

	return nil
}

func (m *volumeMounter) ApplyVolume(ctx context.Context, spec *api.VolumeSpec, onDelete func(*MountVolume) error) (string, *providervolume.Volume, error) {
	plugin, err := m.pluginManager.FindPluginBySpec(spec)
	if err != nil {
		return "", nil, err
	}

	existing, err := m.GetVolume(spec.Name)
	if err != nil && !errors.Is(err, ErrMountedVolumeNotFound) {
		return "", nil, err
	}
	if err == nil && existing.PluginName != plugin.Name() {
		if err := onDelete(existing); err != nil {
			return "", nil, err
		}

		if err := m.DeleteVolume(ctx, spec.Name); err != nil {
			return "", nil, err
		}
	}

	volumeID, err := plugin.GetBackingVolumeID(spec)
	if err != nil {
		return "", nil, err
	}

	volume, err := plugin.Apply(ctx, spec, m.machine)
	if err != nil {
		return "", nil, err
	}

	return GetUniqueVolumeName(plugin.Name(), volumeID), volume, nil
}

func (r *MachineReconciler) applyVolume(
	ctx context.Context,
	log logr.Logger,
	machine *api.Machine,
	desiredVolume *api.VolumeSpec,
	mountedVolumes VolumeMounter,
	attacher VolumeAttacher,
) (string, int64, error) {
	log.V(1).Info("Getting volume spec")

	log.V(1).Info("Applying volume")
	volumeID, providerVolume, err := mountedVolumes.ApplyVolume(ctx, desiredVolume, func(outdated *MountVolume) error {
		log.V(1).Info("Detaching outdated mounted volume before deleting", "PluginName", outdated.PluginName)
		if err := attacher.DetachVolume(outdated.ComputeVolumeName); err != nil && !errors.Is(err, ErrAttachedVolumeNotFound) {
			return fmt.Errorf("error detaching volume: %w", err)
		}
		return nil
	})
	if err != nil {
		return "", 0, fmt.Errorf("error applying volume mount: %w", err)
	}

	log.V(1).Info("Ensuring volume is attached")
	if err := attacher.AttachVolume(&AttachVolume{
		Name:   desiredVolume.Name,
		Device: desiredVolume.Device,
		Spec:   *providerVolume,
	}); err != nil && !errors.Is(err, ErrAttachedVolumeAlreadyExists) {
		return "", 0, fmt.Errorf("error ensuring volume is attached: %w", err)
	}

	//TODO do epsilon comparison
	if lastVolumeSize := getLastVolumeSize(machine, volumeID); lastVolumeSize != 0 && providerVolume.Size != lastVolumeSize {
		log.V(1).Info("Resize volume", "volumeID", volumeID, "lastSize", lastVolumeSize, "volumeSize", providerVolume.Size)
		if err := attacher.ResizeVolume(&AttachVolume{
			Name:   desiredVolume.Name,
			Device: desiredVolume.Device,
			Spec:   *providerVolume,
		}); err != nil {
			return "", 0, fmt.Errorf("failed to resize volume: %w", err)
		}
	}

	return volumeID, providerVolume.Size, nil
}

func (r *MachineReconciler) listDesiredVolumes(machine *api.Machine) map[string]*api.VolumeSpec {
	res := make(map[string]*api.VolumeSpec)

	for _, volume := range machine.Spec.Volumes {
		res[volume.Name] = volume
	}
	return res
}

const (
	volumeAliasPrefix = "ua-volume-"
)

func isDiskAlias(alias string) bool {
	return strings.HasPrefix(alias, volumeAliasPrefix)
}

func parseVolumeDiskAlias(alias string) (computeVolumeName string, err error) {
	if !strings.HasPrefix(alias, volumeAliasPrefix) {
		return "", fmt.Errorf("no disk volume alias: %s", alias)
	}

	data := strings.TrimPrefix(alias, volumeAliasPrefix)
	res, err := base64.RawURLEncoding.DecodeString(data)
	if err != nil {
		return "", err
	}

	return string(res), nil
}

func volumeDiskAlias(computeVolumeName string) string {
	data := base64.RawURLEncoding.EncodeToString([]byte(computeVolumeName))
	return fmt.Sprintf("%s%s", volumeAliasPrefix, data)
}

func (a *libvirtVolumeAttacher) secretUUID(computeVolumeName string) string {
	return uuid.NewHash(sha256.New(), uuid.Nil, []byte(fmt.Sprintf("%s/%s", a.domainDesc.UUID, computeVolumeName)), 5).String()
}

func (a *libvirtVolumeAttacher) secretEncryptionUUID(computeVolumeName string) string {
	return uuid.NewHash(sha256.New(), uuid.Nil, []byte(fmt.Sprintf("enc/%s/%s", a.domainDesc.UUID, computeVolumeName)), 5).String()
}

func (a *libvirtVolumeAttacher) providerVolumeToLibvirt(computeVolumeName string, vol *providervolume.Volume, dev string) (*libvirtxml.DomainDisk, *libvirtxml.Secret, *libvirtxml.Secret, []byte, []byte, error) {
	deviceName := computeVirtioDiskTargetDeviceName(dev)

	disk := &libvirtxml.DomainDisk{
		Alias: &libvirtxml.DomainAlias{
			Name: volumeDiskAlias(computeVolumeName),
		},
		Device: "disk",
		Target: &libvirtxml.DomainDiskTarget{
			Dev: deviceName,
			Bus: "virtio",
		},
		Serial: dev + "-" + vol.Handle,
	}

	switch {
	case vol.QCow2File != "":
		disk.Driver = &libvirtxml.DomainDiskDriver{
			Name: "qemu",
			Type: "qcow2",
		}
		disk.Source = &libvirtxml.DomainDiskSource{
			File: &libvirtxml.DomainDiskSourceFile{
				File: vol.QCow2File,
			},
		}
		return disk, nil, nil, nil, nil, nil
	case vol.RawFile != "":
		disk.Driver = &libvirtxml.DomainDiskDriver{
			Name: "qemu",
			Type: "raw",
		}
		disk.Source = &libvirtxml.DomainDiskSource{
			File: &libvirtxml.DomainDiskSourceFile{
				File: vol.RawFile,
			},
		}
		return disk, nil, nil, nil, nil, nil
	case vol.CephDisk != nil:
		var (
			secret                *libvirtxml.Secret
			secretValue           []byte
			diskAuth              *libvirtxml.DomainDiskAuth
			encryptionSecret      *libvirtxml.Secret
			encryptionSecretValue []byte
			diskEncryption        *libvirtxml.DomainDiskEncryption
		)

		if auth := vol.CephDisk.Auth; auth != nil {
			diskAuth = &libvirtxml.DomainDiskAuth{
				Username: auth.UserName,
				Secret: &libvirtxml.DomainDiskSecret{
					Type: "ceph",
					UUID: a.secretUUID(computeVolumeName),
				},
			}

			secret = &libvirtxml.Secret{
				Ephemeral: "no",
				Private:   "no",
				UUID:      a.secretUUID(computeVolumeName),
				Usage: &libvirtxml.SecretUsage{
					Type: "ceph",
					Name: fmt.Sprintf("domain.%s.volume.%s.client.%s secret", a.domainDesc.UUID, computeVolumeName, auth.UserName),
				},
			}

			// Libvirt APIs expect the ceph user key (which usually can be used directly as base64)
			// to be decoded, hence the decoding again here.
			var err error
			secretValue, err = base64.StdEncoding.DecodeString(auth.UserKey)
			if err != nil {
				return nil, nil, nil, nil, nil, fmt.Errorf("error decoding ceph user key: %w", err)
			}
		}

		if encryption := vol.CephDisk.Encryption; encryption != nil && encryption.EncryptionKey != "" {
			diskEncryption = &libvirtxml.DomainDiskEncryption{
				Format: "luks2",
				Engine: "librbd",
				Secrets: []libvirtxml.DomainDiskSecret{
					{
						Type: "passphrase",
						UUID: a.secretEncryptionUUID(computeVolumeName),
					},
				},
			}

			encryptionSecret = &libvirtxml.Secret{
				Ephemeral: "no",
				Private:   "yes",
				UUID:      a.secretEncryptionUUID(computeVolumeName),
				Usage: &libvirtxml.SecretUsage{
					Type:   "volume",
					Name:   fmt.Sprintf("domain.%s.volume.%s secret", a.domainDesc.UUID, computeVolumeName),
					Volume: fmt.Sprintf("domain.%s.volume.%s", a.domainDesc.UUID, computeVolumeName),
				},
			}

			encryptionSecretValue = []byte(encryption.EncryptionKey)
		}

		hosts := make([]libvirtxml.DomainDiskSourceHost, 0, len(vol.CephDisk.Monitors))
		for _, monitor := range vol.CephDisk.Monitors {
			hosts = append(hosts, libvirtxml.DomainDiskSourceHost{
				Name: monitor.Name,
				Port: monitor.Port,
			})
		}
		disk.Source = &libvirtxml.DomainDiskSource{
			Network: &libvirtxml.DomainDiskSourceNetwork{
				Protocol: "rbd",
				Name:     vol.CephDisk.Name,
				Hosts:    hosts,
				Auth:     diskAuth,
			},
			Encryption: diskEncryption,
		}
		disk.Driver = &libvirtxml.DomainDiskDriver{
			Cache: a.volumeCachePolicy,
			IO:    "threads",
		}

		return disk, secret, encryptionSecret, secretValue, encryptionSecretValue, nil
	default:
		return nil, nil, nil, nil, nil, fmt.Errorf("unsupported provider volume %#+v", vol)
	}
}

func libvirtDiskToProviderVolume(disk *libvirtxml.DomainDisk) (*providervolume.Volume, error) {
	src := disk.Source
	if src == nil {
		return nil, fmt.Errorf("disk does not specify a source")
	}

	switch {
	case disk.Driver != nil && disk.Driver.Name == "qemu" && disk.Driver.Type == "qcow2" && src.File != nil && src.File.File != "":
		return &providervolume.Volume{
			QCow2File: src.File.File,
		}, nil
	case disk.Driver != nil && disk.Driver.Name == "qemu" && disk.Driver.Type == "raw" && src.File != nil && src.File.File != "":
		return &providervolume.Volume{
			RawFile: src.File.File,
		}, nil
	case src.Network != nil && src.Network.Protocol == "rbd":
		netSrc := src.Network
		monitors := make([]providervolume.CephMonitor, 0, len(netSrc.Hosts))
		for _, host := range netSrc.Hosts {
			monitors = append(monitors, providervolume.CephMonitor{Name: host.Name, Port: host.Port})
		}

		return &providervolume.Volume{
			CephDisk: &providervolume.CephDisk{
				Name:     netSrc.Name,
				Monitors: monitors,
				// TODO: Check whether it's necessary to reconstruct Auth.
			},
		}, nil
	default:
		return nil, fmt.Errorf("cannot determine volume from disk %#+v", disk)
	}
}
