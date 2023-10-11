// Copyright 2023 OnMetal authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package controllers

import (
	"context"
	"errors"
	"fmt"
	"github.com/digitalocean/go-libvirt"
	"github.com/go-logr/logr"
	"github.com/onmetal/libvirt-driver/pkg/api"
	"github.com/onmetal/libvirt-driver/pkg/event"
	virtletimage "github.com/onmetal/libvirt-driver/pkg/image"
	"github.com/onmetal/libvirt-driver/pkg/libvirt/guest"
	"github.com/onmetal/libvirt-driver/pkg/os/osutils"
	"github.com/onmetal/libvirt-driver/pkg/raw"
	"github.com/onmetal/libvirt-driver/pkg/store"
	"github.com/onmetal/libvirt-driver/pkg/utils"
	virtlethost "github.com/onmetal/libvirt-driver/pkg/virtlethost" // TODO: Change to a better naming for all imports, libvirthost?
	"github.com/onmetal/virtlet/libvirt/libvirtutils"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/utils/pointer"
	"libvirt.org/go/libvirtxml"
	"os"
	"slices"
	"sync"
)

const (
	MachineFinalizer                = "machine"
	filePerm                        = 0666
	rootFSAlias                     = "ua-rootfs"
	libvirtDomainXMLIgnitionKeyName = "opt/com.coreos/config"
)

type MachineReconcilerOptions struct {
	GuestCapabilities guest.Capabilities
	TCMallocLibPath   string
	ImageCache        virtletimage.Cache
	Raw               raw.Raw
	Host              virtlethost.Host
}

func NewMachineReconciler(
	log logr.Logger,
	libvirt *libvirt.Libvirt,
	machines store.Store[*api.Machine],
	machineEvents event.Source[*api.Machine],
	volumes store.Store[*api.Volume],
	opts MachineReconcilerOptions,
) (*MachineReconciler, error) {
	//if libvirt == nil {
	//	return nil, fmt.Errorf("must specify libvirt client")
	//}

	if machines == nil {
		return nil, fmt.Errorf("must specify machine store")
	}

	if machineEvents == nil {
		return nil, fmt.Errorf("must specify machine events")
	}

	if volumes == nil {
		return nil, fmt.Errorf("must specify volume store")
	}

	return &MachineReconciler{
		log:               log,
		queue:             workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter()),
		libvirt:           libvirt,
		machines:          machines,
		machineEvents:     machineEvents,
		volumes:           volumes,
		guestCapabilities: opts.GuestCapabilities,
		tcMallocLibPath:   opts.TCMallocLibPath,
		host:              opts.Host,
		imageCache:        opts.ImageCache,
		raw:               opts.Raw,
	}, nil
}

type MachineReconciler struct {
	log   logr.Logger
	queue workqueue.RateLimitingInterface

	libvirt           *libvirt.Libvirt
	guestCapabilities guest.Capabilities
	tcMallocLibPath   string
	host              virtlethost.Host
	imageCache        virtletimage.Cache
	raw               raw.Raw

	machines      store.Store[*api.Machine]
	machineEvents event.Source[*api.Machine]

	volumes store.Store[*api.Volume]
}

func (r *MachineReconciler) Start(ctx context.Context) error {
	log := r.log

	//todo make configurable
	workerSize := 15

	imgEventReg, err := r.machineEvents.AddHandler(event.HandlerFunc[*api.Machine](func(evt event.Event[*api.Machine]) {
		r.queue.Add(evt.Object.ID)
	}))
	if err != nil {
		return err
	}
	defer func() {
		if err = r.machineEvents.RemoveHandler(imgEventReg); err != nil {
			log.Error(err, "failed to remove machine event handler")
		}
	}()

	go func() {
		<-ctx.Done()
		r.queue.ShutDown()
	}()

	var wg sync.WaitGroup
	for i := 0; i < workerSize; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for r.processNextWorkItem(ctx, log) {
			}
		}()
	}

	wg.Wait()
	return nil
}

func (r *MachineReconciler) processNextWorkItem(ctx context.Context, log logr.Logger) bool {
	item, shutdown := r.queue.Get()
	if shutdown {
		return false
	}
	defer r.queue.Done(item)

	id := item.(string)
	log = log.WithValues("machineId", id)
	ctx = logr.NewContext(ctx, log)

	if err := r.reconcileMachine(ctx, id); err != nil {
		log.Error(err, "failed to reconcile machine")
		r.queue.AddRateLimited(item)
		return true
	}

	r.queue.Forget(item)
	return true
}

func (r *MachineReconciler) reconcileMachine(ctx context.Context, id string) error {
	log := logr.FromContextOrDiscard(ctx)

	log.V(2).Info("Getting machine from store", "id", id)
	machine, err := r.machines.Get(ctx, id)
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("failed to fetch machine from store: %w", err)
		}

		return nil
	}

	if machine.DeletedAt != nil {
		if err := r.deleteMachine(ctx, log, machine); err != nil {
			return fmt.Errorf("failed to delete machine: %w", err)
		}
		log.V(1).Info("Successfully deleted machine")
		return nil
	}

	log.V(1).Info("Looking up domain")
	if _, err := r.libvirt.DomainLookupByUUID(libvirtutils.UUIDStringToBytes(machine.GetID())); err != nil {
		if !libvirt.IsNotFound(err) {
			return fmt.Errorf("error getting domain %s: %w", machine.GetID(), err)
		}

		log.V(1).Info("Creating new domain")
		_, _, err := r.createDomain(ctx, log, machine)
		if err != nil {
			return fmt.Errorf("error creating the domain %s: %w", machine.GetID(), err)
		}

		log.V(1).Info("Created domain")
		return nil /// TODO: Check how to handle the NIC/Volume States here better.
		//return computev1alpha1.MachineStatePending, volumeStates, nicStates, nil
	}

	if !slices.Contains(machine.Finalizers, MachineFinalizer) {
		machine.Finalizers = append(machine.Finalizers, MachineFinalizer)
		if _, err := r.machines.Update(ctx, machine); err != nil {
			return fmt.Errorf("failed to set finalizers: %w", err)
		}
		return nil
	}

	return nil
}

func (r *MachineReconciler) deleteMachine(ctx context.Context, log logr.Logger, machine *api.Machine) error {
	if !slices.Contains(machine.Finalizers, MachineFinalizer) {
		log.V(1).Info("machine has no finalizer: done")
		return nil
	}

	//do libvirt cleanup

	machine.Finalizers = utils.DeleteSliceElement(machine.Finalizers, MachineFinalizer)
	if _, err := r.machines.Update(ctx, machine); store.IgnoreErrNotFound(err) != nil {
		return fmt.Errorf("failed to update machine metadata: %w", err)
	}
	log.V(2).Info("Removed Finalizers")

	return nil
}

func (r *MachineReconciler) createDomain(
	ctx context.Context,
	log logr.Logger,
	machine *api.Machine,
) ([]api.VolumeStatus, []api.NetworkInterfaceStatus, error) { // TODO add NetworkInterfaceStatus
	domainXML, volumeStates, nicStates, err := r.domainFor(ctx, log, machine)
	if err != nil {
		return nil, nil, err
	}

	domainXMLData, err := domainXML.Marshal()
	if err != nil {
		return nil, nil, err
	}

	log.V(1).Info("Creating domain")
	log.V(2).Info("Domain", "XML", domainXMLData)
	if _, err := r.libvirt.DomainCreateXML(domainXMLData, libvirt.DomainNone); err != nil {
		return nil, nil, err
	}

	return volumeStates, nicStates, nil
}

func (r *MachineReconciler) domainFor(
	ctx context.Context,
	log logr.Logger,
	machine *api.Machine,
) (*libvirtxml.Domain, []api.VolumeStatus, []api.NetworkInterfaceStatus, error) {
	architecture := "x86_64"  // TODO: Detect this from the image / machine specification.
	osType := guest.OSTypeHVM // TODO: Make this configurable via machine class
	domainSettings, err := r.guestCapabilities.SettingsFor(guest.Requests{
		Architecture: architecture,
		OSType:       osType,
	})
	if err != nil {
		return nil, nil, nil, err
	}

	domainDesc := &libvirtxml.Domain{
		Name:       machine.GetID(),
		UUID:       machine.GetID(),
		Type:       domainSettings.Type,
		OnPoweroff: "destroy",
		OnReboot:   "restart",
		OnCrash:    "coredump-restart",
		CPU: &libvirtxml.DomainCPU{
			Mode: "host-passthrough",
		},
		Features: &libvirtxml.DomainFeatureList{
			ACPI: &libvirtxml.DomainFeature{},
			APIC: &libvirtxml.DomainFeatureAPIC{},
		},
		OS: &libvirtxml.DomainOS{
			Type: &libvirtxml.DomainOSType{
				Type:    string(osType),
				Arch:    architecture,
				Machine: domainSettings.Machine,
			},
			BootDevices: []libvirtxml.DomainBootDevice{
				{Dev: "hd"},
			},
			Firmware: "efi",
			FirmwareInfo: &libvirtxml.DomainOSFirmwareInfo{
				Features: []libvirtxml.DomainOSFirmwareFeature{
					{
						Name:    "secure-boot",
						Enabled: "no",
					},
				},
			},
		},
		Clock: &libvirtxml.DomainClock{
			Offset: "utc",
			Timer: []libvirtxml.DomainTimer{
				{
					Name:       "rtc",
					TickPolicy: "catchup",
				},
				{
					Name:       "hpet",
					TickPolicy: "catchup",
				},
				{
					Name:       "tsc",
					Mode:       "paravirt",
					TickPolicy: "catchup",
				},
			},
		},
		Devices: &libvirtxml.DomainDeviceList{
			Serials: []libvirtxml.DomainSerial{
				{
					Target: &libvirtxml.DomainSerialTarget{
						Type: "pci-serial",
					},
				},
			},
			Consoles: []libvirtxml.DomainConsole{
				{
					TTY: "pty",
					Target: &libvirtxml.DomainConsoleTarget{
						Type: "serial",
					},
				},
			},
			//Watchdog: &libvirtxml.DomainWatchdog{  // TODO: Add Watchdog again with proper libvirt version
			//	Model:  "i6300esb",
			//	Action: "reset",
			//},
			RNGs: []libvirtxml.DomainRNG{
				{
					Model: "virtio",
					Rate: &libvirtxml.DomainRNGRate{
						Bytes: 512,
					},
					Backend: &libvirtxml.DomainRNGBackend{
						Random: &libvirtxml.DomainRNGBackendRandom{},
					},
				},
			},
		},
	}

	if err := r.setDomainResources(ctx, log, machine, domainDesc); err != nil {
		return nil, nil, nil, err
	}

	if err := r.setDomainPCIControllers(machine, domainDesc); err != nil {
		return nil, nil, nil, err
	}

	if err := r.setTCMallocPath(machine, domainDesc); err != nil {
		return nil, nil, nil, err
	}

	if machineImgRef := machine.Spec.Image; machineImgRef != nil && pointer.StringDeref(machineImgRef, "") != "" {
		if err := r.setDomainImage(ctx, machine, domainDesc, pointer.StringDeref(machineImgRef, "")); err != nil {
			return nil, nil, nil, err
		}
	}

	if ignitionSpec := machine.Spec.Ignition; ignitionSpec != nil {
		if err := r.setDomainIgnition(ctx, machine, domainDesc); err != nil {
			return nil, nil, nil, err
		}
	}

	var desiredVolumes []*api.Volume
	for _, volumeID := range machine.Spec.Volumes {
		volume, err := r.volumes.Get(ctx, volumeID)
		if err != nil {
			//	TODO

		}
		desiredVolumes = append(desiredVolumes, volume)
	}

	return domainDesc, nil, nil, nil
}

func (r *MachineReconciler) setDomainResources(ctx context.Context, log logr.Logger, machine *api.Machine, domain *libvirtxml.Domain) error {
	// TODO: check if there is better or check possible while conversion to uint
	domain.Memory = &libvirtxml.DomainMemory{
		Value: uint(machine.Spec.MemoryBytes),
		Unit:  "Byte",
	}
	domain.VCPU = &libvirtxml.DomainVCPU{
		Value: uint(machine.Spec.CpuMillis),
	}
	return nil
}

// TODO: Investigate hotplugging the pcie-root-port controllers with disks.
// Ref: https://libvirt.org/pci-hotplug.html#x86_64-q35
func (r *MachineReconciler) setDomainPCIControllers(machine *api.Machine, domain *libvirtxml.Domain) error {
	domain.Devices.Controllers = append(domain.Devices.Controllers, libvirtxml.DomainController{
		Type:  "pci",
		Model: "pcie-root",
	})

	for i := 1; i <= 30; i++ {
		domain.Devices.Controllers = append(domain.Devices.Controllers, libvirtxml.DomainController{
			Type:  "pci",
			Model: "pcie-root-port",
		})
	}
	return nil
}

// setTCMallocPath enables support for the tcmalloc for the VMs.
func (r *MachineReconciler) setTCMallocPath(machine *api.Machine, domain *libvirtxml.Domain) error {
	if r.tcMallocLibPath == "" {
		return nil
	}

	if domain.QEMUCommandline == nil {
		domain.QEMUCommandline = &libvirtxml.DomainQEMUCommandline{}
	}
	domain.QEMUCommandline.Envs = append(domain.QEMUCommandline.Envs, libvirtxml.DomainQEMUCommandlineEnv{
		Name:  "LD_PRELOAD",
		Value: r.tcMallocLibPath,
	})
	return nil
}

func (r *MachineReconciler) setDomainImage(
	ctx context.Context,
	machine *api.Machine,
	domain *libvirtxml.Domain,
	machineImgRef string,
) error {
	img, err := r.imageCache.Get(ctx, machineImgRef)
	if err != nil {
		if !errors.Is(err, virtletimage.ErrImagePulling) {
			return err
		}
		return err
	}

	rootFSFile := r.host.MachineRootFSFile(types.UID(machine.GetID()))
	ok, err := osutils.RegularFileExists(rootFSFile)
	if err != nil {
		return err
	}
	if !ok {
		if err := r.raw.Create(rootFSFile, raw.WithSourceFile(img.RootFS.Path)); err != nil {
			return fmt.Errorf("error creating root fs disk: %w", err)
		}
		if err := os.Chmod(rootFSFile, filePerm); err != nil {
			return fmt.Errorf("error changing root fs disk mode: %w", err)
		}
	}

	domain.OS.Kernel = img.Kernel.Path
	domain.OS.Initrd = img.InitRAMFs.Path
	domain.OS.Cmdline = img.Config.CommandLine
	domain.Devices.Disks = append(domain.Devices.Disks, libvirtxml.DomainDisk{
		Alias: &libvirtxml.DomainAlias{
			Name: rootFSAlias,
		},
		Device: "disk",
		Driver: &libvirtxml.DomainDiskDriver{
			Name: "qemu",
			Type: "raw",
		},
		Source: &libvirtxml.DomainDiskSource{
			File: &libvirtxml.DomainDiskSourceFile{
				File: rootFSFile,
			},
		},
		Target: &libvirtxml.DomainDiskTarget{
			Dev: "vdaaa", // TODO: Reserving vdaaa for ramdisk, so that it doesnt conflict with other volumes, investigate better solution.
			Bus: "virtio",
		},
		Serial:   "machineboot",
		ReadOnly: &libvirtxml.DomainDiskReadOnly{},
	})
	return nil
}

func (r *MachineReconciler) setDomainIgnition(ctx context.Context, machine *api.Machine, domain *libvirtxml.Domain) error {
	if machine.Spec.Ignition == nil {
		return fmt.Errorf("no IgnitionData found in the Machine %s", machine.GetID())
	}
	ignitionData := machine.Spec.Ignition

	ignPath := r.host.MachineIgnitionFile(types.UID(machine.GetID()))
	if err := os.WriteFile(ignPath, ignitionData, filePerm); err != nil {
		return err
	}

	domain.SysInfo = append(domain.SysInfo, libvirtxml.DomainSysInfo{
		FWCfg: &libvirtxml.DomainSysInfoFWCfg{
			Entry: []libvirtxml.DomainSysInfoEntry{
				{
					// TODO: Make the ignition sysinfo key configurable via onmetal-image / machine spec.
					Name: libvirtDomainXMLIgnitionKeyName,
					File: ignPath,
				},
			},
		},
	})
	return nil
}

func (r *MachineReconciler) getDomainDesc(machineUID types.UID) (*libvirtxml.Domain, error) {
	domainXMLData, err := r.libvirt.DomainGetXMLDesc(libvirt.Domain{UUID: libvirtutils.UUIDStringToBytes(string(machineUID))}, 0)
	if err != nil {
		return nil, err
	}

	domainXML := &libvirtxml.Domain{}
	if err := domainXML.Unmarshal(domainXMLData); err != nil {
		return nil, err
	}
	return domainXML, nil
}
