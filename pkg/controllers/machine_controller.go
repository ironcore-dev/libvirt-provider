// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controllers

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/digitalocean/go-libvirt"
	"github.com/go-logr/logr"
	"github.com/ironcore-dev/libvirt-provider/pkg/api"
	"github.com/ironcore-dev/libvirt-provider/pkg/event"
	providerimage "github.com/ironcore-dev/libvirt-provider/pkg/image"
	"github.com/ironcore-dev/libvirt-provider/pkg/libvirt/guest"
	libvirtutils "github.com/ironcore-dev/libvirt-provider/pkg/libvirt/utils"
	providermeta "github.com/ironcore-dev/libvirt-provider/pkg/meta"
	"github.com/ironcore-dev/libvirt-provider/pkg/os/osutils"
	providernetworkinterface "github.com/ironcore-dev/libvirt-provider/pkg/plugins/networkinterface"
	providervolume "github.com/ironcore-dev/libvirt-provider/pkg/plugins/volume"
	providerhost "github.com/ironcore-dev/libvirt-provider/pkg/providerhost"
	"github.com/ironcore-dev/libvirt-provider/pkg/raw"
	"github.com/ironcore-dev/libvirt-provider/pkg/store"
	"github.com/ironcore-dev/libvirt-provider/pkg/utils"
	machinev1alpha1 "github.com/ironcore-dev/libvirt-provider/provider/api/v1alpha1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/utils/ptr"
	"libvirt.org/go/libvirtxml"
)

const (
	MachineFinalizer                = "machine"
	filePerm                        = 0666
	rootFSAlias                     = "ua-rootfs"
	libvirtDomainXMLIgnitionKeyName = "opt/com.coreos/config"
	networkInterfaceAliasPrefix     = "ua-networkinterface-"
)

var (
	// TODO: improve domainStateToMachineState since some states are mapped to computev1alpha1.MachineStatePending
	// where it doesn't make that much sense.
	domainStateToMachineState = map[libvirt.DomainState]api.MachineState{
		libvirt.DomainNostate:  api.MachineStatePending,
		libvirt.DomainRunning:  api.MachineStateRunning,
		libvirt.DomainBlocked:  api.MachineStatePending,
		libvirt.DomainPaused:   api.MachineStatePending,
		libvirt.DomainShutdown: api.MachineStateTerminating,
		// it isn't probably supported by transient domain
		libvirt.DomainShutoff:     api.MachineStateTerminated,
		libvirt.DomainPmsuspended: api.MachineStatePending,
	}
)

type MachineReconcilerOptions struct {
	GuestCapabilities              guest.Capabilities
	TCMallocLibPath                string
	ImageCache                     providerimage.Cache
	Raw                            raw.Raw
	Host                           providerhost.Host
	VolumePluginManager            *providervolume.PluginManager
	NetworkInterfacePlugin         providernetworkinterface.Plugin
	VolumeEvents                   event.Source[*api.Machine]
	ResyncIntervalVolumeSize       time.Duration
	ResyncIntervalGarbageCollector time.Duration
	EnableHugepages                bool
	GCVMGracefulShutdownTimeout    time.Duration
}

func NewMachineReconciler(
	log logr.Logger,
	libvirt *libvirt.Libvirt,
	machines store.Store[*api.Machine],
	machineEvents event.Source[*api.Machine],
	opts MachineReconcilerOptions,
) (*MachineReconciler, error) {
	if libvirt == nil {
		return nil, fmt.Errorf("must specify libvirt client")
	}

	if machines == nil {
		return nil, fmt.Errorf("must specify machine store")
	}

	if machineEvents == nil {
		return nil, fmt.Errorf("must specify machine events")
	}

	return &MachineReconciler{
		log:                            log,
		queue:                          workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter()),
		libvirt:                        libvirt,
		machines:                       machines,
		machineEvents:                  machineEvents,
		guestCapabilities:              opts.GuestCapabilities,
		tcMallocLibPath:                opts.TCMallocLibPath,
		host:                           opts.Host,
		imageCache:                     opts.ImageCache,
		raw:                            opts.Raw,
		volumePluginManager:            opts.VolumePluginManager,
		networkInterfacePlugin:         opts.NetworkInterfacePlugin,
		resyncIntervalVolumeSize:       opts.ResyncIntervalVolumeSize,
		resyncIntervalGarbageCollector: opts.ResyncIntervalGarbageCollector,
		enableHugepages:                opts.EnableHugepages,
		gcVMGracefulShutdownTimeout:    opts.GCVMGracefulShutdownTimeout,
	}, nil
}

type MachineReconciler struct {
	log   logr.Logger
	queue workqueue.RateLimitingInterface

	libvirt           *libvirt.Libvirt
	guestCapabilities guest.Capabilities
	tcMallocLibPath   string
	host              providerhost.Host
	imageCache        providerimage.Cache
	raw               raw.Raw

	enableHugepages bool

	volumePluginManager    *providervolume.PluginManager
	networkInterfacePlugin providernetworkinterface.Plugin

	machines      store.Store[*api.Machine]
	machineEvents event.Source[*api.Machine]

	resyncIntervalVolumeSize time.Duration

	gcVMGracefulShutdownTimeout    time.Duration
	resyncIntervalGarbageCollector time.Duration
}

func (r *MachineReconciler) Start(ctx context.Context) error {
	log := r.log

	//todo make configurable
	workerSize := 15

	r.imageCache.AddListener(providerimage.ListenerFuncs{
		HandlePullDoneFunc: func(evt providerimage.PullDoneEvent) {
			machines, err := r.machines.List(ctx)
			if err != nil {
				log.Error(err, "failed to list machine")
				return
			}

			for _, machine := range machines {
				if ptr.Deref(machine.Spec.Image, "") == evt.Ref {
					log.V(1).Info("Image pulled: Requeue machines", "Image", evt.Ref, "Machine", machine.ID)
					r.queue.Add(machine.ID)
				}
			}
		},
	})

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

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		r.startCheckAndEnqueueVolumeResize(ctx, r.log.WithName("volume-size"))
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		r.startEnqueueMachineByLibvirtEvent(ctx, r.log.WithName("libvirt-event"))
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		r.startGarbageCollector(ctx, r.log.WithName("garbage-collector"))
	}()

	go func() {
		<-ctx.Done()
		r.queue.ShutDown()
	}()

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

func (r *MachineReconciler) startCheckAndEnqueueVolumeResize(ctx context.Context, log logr.Logger) {
	wait.UntilWithContext(ctx, func(ctx context.Context) {
		machines, err := r.machines.List(ctx)
		if err != nil {
			log.Error(err, "failed to list machines")
			return
		}

		for _, machine := range machines {
			if machine.DeletedAt != nil || !slices.Contains(machine.Finalizers, MachineFinalizer) {
				continue
			}

			var shouldEnqueue bool
			for _, volume := range machine.Spec.Volumes {
				plugin, err := r.volumePluginManager.FindPluginBySpec(volume)
				if err != nil {
					log.Error(err, "failed to get volume plugin", "machineID", machine.ID, "volumeName", volume.Name)
					continue
				}

				volumeID, err := plugin.GetBackingVolumeID(volume)
				if err != nil {
					log.Error(err, "failed to get volume id", "machineID", machine.ID, "volumeName", volume.Name)
					continue
				}

				volumeSize, err := plugin.GetSize(ctx, volume)
				if err != nil {
					log.Error(err, "failed to get volume size", "machineID", machine.ID, "volumeName", volume.Name, "volumeID", volumeID)
					continue
				}

				if lastVolumeSize := getLastVolumeSize(machine, GetUniqueVolumeName(plugin.Name(), volumeID)); volumeSize != ptr.Deref(lastVolumeSize, 0) {
					log.V(1).Info("Volume size changed", "volumeName", volume.Name, "volumeID", volumeID, "machineID", machine.ID, "lastSize", lastVolumeSize, "volumeSize", volumeSize)
					shouldEnqueue = true
					break
				}
			}

			if shouldEnqueue {
				r.queue.AddRateLimited(machine.ID)
			}
		}
	}, r.resyncIntervalVolumeSize)
}

func (r *MachineReconciler) startEnqueueMachineByLibvirtEvent(ctx context.Context, log logr.Logger) {
	lifecycleEvents, err := r.libvirt.LifecycleEvents(ctx)
	if err != nil {
		log.Error(err, "failed to subscribe to libvirt lifecycle events")
		return
	}

	log.Info("Subscribing to libvirt lifecycle events")

	for {
		select {
		case evt, ok := <-lifecycleEvents:
			if !ok {
				log.Error(fmt.Errorf("libvirt lifecycle event channel closed"), "failed to process event")
				return
			}

			machine, err := r.machines.Get(ctx, evt.Dom.Name)
			if err != nil {
				if errors.Is(err, store.ErrNotFound) {
					log.V(2).Info("Skipped: not managed by libvirt-provider", "machineID", machine.ID)
					continue
				}
				log.Error(err, "failed to fetch machine from store")
				continue
			}

			log.V(1).Info("requeue machine", "machineID", machine.ID, "lifecycleEventID", evt.Event)
			r.queue.AddRateLimited(machine.ID)
		case <-ctx.Done():
			log.Info("Context done for libvirt event lifecycle.")
			return
		}
	}
}

func (r *MachineReconciler) startGarbageCollector(ctx context.Context, log logr.Logger) {
	wait.UntilWithContext(ctx, func(ctx context.Context) {
		machines, err := r.machines.List(ctx)
		if err != nil {
			log.Error(err, "failed to list machines")
			return
		}

		for _, machine := range machines {
			if !slices.Contains(machine.Finalizers, MachineFinalizer) || machine.DeletedAt == nil {
				continue
			}

			logger := log.WithValues("machineID", machine.ID)
			if err := r.processMachineDeletion(ctx, logger, machine); err != nil {
				logger.Error(err, "failed to garbage collect machine")
			}
		}

	}, r.resyncIntervalGarbageCollector)
}

func (r *MachineReconciler) processMachineDeletion(ctx context.Context, log logr.Logger, machine *api.Machine) error {
	isDeleting, err := r.deleteMachine(ctx, log, machine)
	switch {
	case isDeleting:
		return nil
	case err != nil:
		return fmt.Errorf("failed to delete machine: %w", err)
	}
	log.V(1).Info("Deleted machine")
	machine.Status.State = api.MachineStateTerminated
	machine, err = r.machines.Update(ctx, machine)
	if err != nil {
		return fmt.Errorf("failed to update machine state: %w", err)
	}

	if err := r.deleteVolumes(ctx, log, machine); err != nil {
		return fmt.Errorf("failed to remove machine disks: %w", err)
	}
	log.V(1).Info("Removed machine disks")

	if err := r.deleteNetworkInterfaces(ctx, log, machine); err != nil {
		return fmt.Errorf("failed to remove machine network interfaces: %w", err)
	}
	log.V(1).Info("Removed network interfaces")

	if err := os.RemoveAll(r.host.MachineDir(machine.ID)); err != nil {
		return fmt.Errorf("failed to remove machine directory: %w", err)
	}
	log.V(1).Info("Removed machine directory")

	machine.Finalizers = utils.DeleteSliceElement(machine.Finalizers, MachineFinalizer)
	if _, err := r.machines.Update(ctx, machine); store.IgnoreErrNotFound(err) != nil {
		return fmt.Errorf("failed to update machine metadata: %w", err)
	}
	log.V(1).Info("Removed Finalizer. Deletion completed")

	return nil
}

func (r *MachineReconciler) deleteMachine(ctx context.Context, log logr.Logger, machine *api.Machine) (bool, error) {
	domain := libvirt.Domain{
		UUID: libvirtutils.UUIDStringToBytes(machine.ID),
	}

	if machine.Spec.ShutdownAt.IsZero() {
		machine.Status.State = api.MachineStateTerminating
		machine.Spec.ShutdownAt = time.Now()
		if _, err := r.machines.Update(ctx, machine); err != nil {
			return false, fmt.Errorf("failed to update ShutdownAt and State: %w", err)
		}
		log.V(1).Info("Updated ShutdownAt and State", "ShutdownAt", machine.Spec.ShutdownAt, "State", machine.Status.State)
	}

	if time.Now().Before(machine.Spec.ShutdownAt.Add(r.gcVMGracefulShutdownTimeout)) {
		// Due to heavy load, the AcpiPowerBtn signal might be missed by the VM.
		// Hence, triggering the machine shutdown until VMGracefulShutdownTimeout is over to ensure its reception.
		return r.shutdownMachine(log, machine, domain)
	}

	return false, r.destroyDomain(log, domain)
}

func (r *MachineReconciler) destroyDomain(log logr.Logger, domain libvirt.Domain) error {
	// DomainDestroyFlags is a blocking operation, and its synchronous nature may pose potential performance issues in the future.
	// During test involving 26 empty disks, the function call took a maximum of 1 second to complete.
	if err := r.libvirt.DomainDestroyFlags(domain, libvirt.DomainDestroyGraceful); err != nil {
		if libvirt.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to initiate forceful shutdown: %w", err)
	}

	log.V(1).Info("Destroyed domain")
	return nil
}

func (r *MachineReconciler) shutdownMachine(log logr.Logger, machine *api.Machine, domain libvirt.Domain) (bool, error) {
	log.V(1).Info("Triggering shutdown", "ShutdownAt", machine.Spec.ShutdownAt)

	shutdownMode := libvirt.DomainShutdownAcpiPowerBtn
	if machine.Spec.GuestAgent == api.GuestAgentQemu {
		shutdownMode = libvirt.DomainShutdownGuestAgent
	}
	if err := r.libvirt.DomainShutdownFlags(domain, shutdownMode); err != nil {
		if libvirt.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to initiate shutdown: %w", err)
	}

	return true, nil
}

func (r *MachineReconciler) processNextWorkItem(ctx context.Context, log logr.Logger) bool {
	item, shutdown := r.queue.Get()
	if shutdown {
		return false
	}
	defer r.queue.Done(item)

	id := item.(string)
	log = log.WithValues("machineID", id)
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
		return nil
	}

	if !slices.Contains(machine.Finalizers, MachineFinalizer) {
		machine.Finalizers = append(machine.Finalizers, MachineFinalizer)
		if _, err := r.machines.Update(ctx, machine); err != nil {
			return fmt.Errorf("failed to set finalizers: %w", err)
		}
		return nil
	}

	log.V(1).Info("Making machine directories")
	if err := providerhost.MakeMachineDirs(r.host, machine.ID); err != nil {
		return fmt.Errorf("error making machine directories: %w", err)
	}
	log.V(1).Info("Successfully made machine directories")

	log.V(1).Info("Reconciling domain")
	state, volumeStates, nicStates, err := r.reconcileDomain(ctx, log, machine)
	if err != nil {
		return providerimage.IgnoreImagePulling(err)
	}
	log.V(1).Info("Reconciled domain")

	machine.Status.VolumeStatus = volumeStates
	machine.Status.NetworkInterfaceStatus = nicStates
	machine.Status.State = state

	if _, err = r.machines.Update(ctx, machine); err != nil {
		return fmt.Errorf("failed to update image metadate: %w", err)
	}

	return nil
}

func (r *MachineReconciler) reconcileDomain(
	ctx context.Context,
	log logr.Logger,
	machine *api.Machine,
) (api.MachineState, []api.VolumeStatus, []api.NetworkInterfaceStatus, error) {
	log.V(1).Info("Looking up domain")
	if _, err := r.libvirt.DomainLookupByUUID(libvirtutils.UUIDStringToBytes(machine.ID)); err != nil {
		if !libvirt.IsNotFound(err) {
			return "", nil, nil, fmt.Errorf("error getting domain %s: %w", machine.ID, err)
		}

		log.V(1).Info("Creating new domain")
		volumeStates, nicStates, err := r.createDomain(ctx, log, machine)
		if err != nil {
			return "", nil, nil, err
		}

		log.V(1).Info("Created domain")
		return api.MachineStatePending, volumeStates, nicStates, nil
	}

	log.V(1).Info("Updating existing domain")
	volumeStates, nicStates, err := r.updateDomain(ctx, log, machine)
	if err != nil {
		return "", nil, nil, err
	}

	state, err := r.getMachineState(machine.ID)
	if err != nil {
		return "", nil, nil, fmt.Errorf("error getting machine state: %w", err)
	}

	return state, volumeStates, nicStates, nil
}

func (r *MachineReconciler) updateDomain(
	ctx context.Context,
	log logr.Logger,
	machine *api.Machine,
) ([]api.VolumeStatus, []api.NetworkInterfaceStatus, error) {
	domainDesc, err := r.getDomainDesc(machine.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("error getting domain description: %w", err)
	}

	attacher, err := NewLibvirtVolumeAttacher(domainDesc, NewRunningDomainExecutor(r.libvirt, machine.ID))
	if err != nil {
		return nil, nil, fmt.Errorf("error construction volume attacher: %w", err)
	}

	volumeStates, err := r.attachDetachVolumes(ctx, log, machine, attacher)
	if err != nil {
		return nil, nil, fmt.Errorf("[volumes] %w", err)
	}

	nicStates, err := r.attachDetachNetworkInterfaces(ctx, log, machine, domainDesc)
	if err != nil {
		return nil, nil, fmt.Errorf("[network interfaces] %w", err)
	}

	return volumeStates, nicStates, nil
}

func (r *MachineReconciler) getMachineState(machineID string) (api.MachineState, error) {
	domainState, _, err := r.libvirt.DomainGetState(machineDomain(machineID), 0)
	if err != nil {
		return "", fmt.Errorf("error getting domain state: %w", err)
	}

	if machineState, ok := domainStateToMachineState[libvirt.DomainState(domainState)]; ok {
		return machineState, nil
	}
	return api.MachineStatePending, nil
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

	if err := r.setDomainMetadata(log, machine, domainDesc); err != nil {
		return nil, nil, nil, err
	}

	if err := r.setDomainResources(machine, domainDesc); err != nil {
		return nil, nil, nil, err
	}

	if err := r.setDomainPCIControllers(domainDesc); err != nil {
		return nil, nil, nil, err
	}

	if err := r.setTCMallocPath(domainDesc); err != nil {
		return nil, nil, nil, err
	}

	if machine.Spec.GuestAgent != api.GuestAgentNone {
		r.setGuestAgent(machine, domainDesc)
	}

	if machineImgRef := machine.Spec.Image; machineImgRef != nil && ptr.Deref(machineImgRef, "") != "" {
		if err := r.setDomainImage(ctx, machine, domainDesc, ptr.Deref(machineImgRef, "")); err != nil {
			return nil, nil, nil, err
		}
	}

	if ignitionSpec := machine.Spec.Ignition; ignitionSpec != nil {
		if err := r.setDomainIgnition(machine, domainDesc); err != nil {
			return nil, nil, nil, err
		}
	}

	attacher, err := NewLibvirtVolumeAttacher(domainDesc, NewCreateDomainExecutor(r.libvirt))
	if err != nil {
		return nil, nil, nil, err
	}

	volumeStates, err := r.attachDetachVolumes(ctx, log, machine, attacher)
	if err != nil {
		return nil, nil, nil, err
	}

	nicStates, err := r.setDomainNetworkInterfaces(ctx, machine, domainDesc)
	if err != nil {
		return nil, nil, nil, err
	}

	return domainDesc, volumeStates, nicStates, nil
}

func (r *MachineReconciler) setDomainMetadata(log logr.Logger, machine *api.Machine, domain *libvirtxml.Domain) error {
	labels, found := machine.Metadata.Annotations[machinev1alpha1.LabelsAnnotation]
	if !found {
		log.V(1).Info("IRI machine labels are not annotated in the API machine")
		return nil
	}
	var irimachineLabels map[string]string
	err := json.Unmarshal([]byte(labels), &irimachineLabels)
	if err != nil {
		return fmt.Errorf("error unmarshalling iri machine labels: %w", err)
	}

	encodedLabels := providermeta.IRIMachineLabelsEncoder(irimachineLabels)

	domainMetadata := &providermeta.LibvirtProviderMetadata{
		IRIMmachineLabels: encodedLabels,
	}

	domainMetadataXML, err := xml.Marshal(domainMetadata)
	if err != nil {
		return err
	}
	domain.Metadata = &libvirtxml.DomainMetadata{
		XML: string(domainMetadataXML),
	}
	return nil
}

func (r *MachineReconciler) setDomainResources(machine *api.Machine, domain *libvirtxml.Domain) error {
	// TODO: check if there is better or check possible while conversion to uint
	domain.Memory = &libvirtxml.DomainMemory{
		Value: uint(machine.Spec.MemoryBytes),
		Unit:  "Byte",
	}

	if r.enableHugepages {
		domain.MemoryBacking = &libvirtxml.DomainMemoryBacking{
			MemoryHugePages: &libvirtxml.DomainMemoryHugepages{},
		}
	}

	cpu := uint(machine.Spec.CpuMillis / 1000)
	domain.VCPU = &libvirtxml.DomainVCPU{
		Value: cpu,
	}

	return nil
}

// TODO: Investigate hotplugging the pcie-root-port controllers with disks.
// Ref: https://libvirt.org/pci-hotplug.html#x86_64-q35
func (r *MachineReconciler) setDomainPCIControllers(domain *libvirtxml.Domain) error {
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
func (r *MachineReconciler) setTCMallocPath(domain *libvirtxml.Domain) error {
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

func (r *MachineReconciler) setGuestAgent(machine *api.Machine, domainDesc *libvirtxml.Domain) {
	if domainDesc.Devices == nil {
		domainDesc.Devices = &libvirtxml.DomainDeviceList{}
	}

	if domainDesc.Devices.Channels == nil {
		domainDesc.Devices.Channels = make([]libvirtxml.DomainChannel, 0, 1)
	}

	socketPath := filepath.Join(r.host.MachineDir(machine.GetID()), "qemu-guest-agent.sock")
	agent := libvirtxml.DomainChannel{
		Source: &libvirtxml.DomainChardevSource{
			UNIX: &libvirtxml.DomainChardevSourceUNIX{
				Mode: "bind",
				Path: socketPath,
			},
		},
		Target: &libvirtxml.DomainChannelTarget{
			VirtIO: &libvirtxml.DomainChannelTargetVirtIO{
				Name: "org.qemu.guest_agent.0",
			},
		},
	}

	domainDesc.Devices.Channels = append(domainDesc.Devices.Channels, agent)
	machine.Status.GuestAgentStatus = &api.GuestAgentStatus{Addr: "unix://" + socketPath}
}

func (r *MachineReconciler) setDomainImage(
	ctx context.Context,
	machine *api.Machine,
	domain *libvirtxml.Domain,
	machineImgRef string,
) error {
	img, err := r.imageCache.Get(ctx, machineImgRef)
	if err != nil {
		if !errors.Is(err, providerimage.ErrImagePulling) {
			return err
		}
		return err
	}

	rootFSFile := r.host.MachineRootFSFile(machine.ID)
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

func (r *MachineReconciler) setDomainIgnition(machine *api.Machine, domain *libvirtxml.Domain) error {
	if machine.Spec.Ignition == nil {
		return fmt.Errorf("no IgnitionData found in the Machine %s", machine.GetID())
	}
	ignitionData := machine.Spec.Ignition

	ignPath := r.host.MachineIgnitionFile(machine.ID)
	if err := os.WriteFile(ignPath, ignitionData, filePerm); err != nil {
		return err
	}

	domain.SysInfo = append(domain.SysInfo, libvirtxml.DomainSysInfo{
		FWCfg: &libvirtxml.DomainSysInfoFWCfg{
			Entry: []libvirtxml.DomainSysInfoEntry{
				{
					// TODO: Make the ignition sysinfo key configurable via ironcore-image / machine spec.
					Name: libvirtDomainXMLIgnitionKeyName,
					File: ignPath,
				},
			},
		},
	})
	return nil
}

func (r *MachineReconciler) getDomainDesc(machineID string) (*libvirtxml.Domain, error) {
	domainXMLData, err := r.libvirt.DomainGetXMLDesc(libvirt.Domain{UUID: libvirtutils.UUIDStringToBytes(machineID)}, 0)
	if err != nil {
		return nil, err
	}

	domainXML := &libvirtxml.Domain{}
	if err := domainXML.Unmarshal(domainXMLData); err != nil {
		return nil, err
	}
	return domainXML, nil
}

func machineDomain(machineID string) libvirt.Domain {
	return libvirt.Domain{
		UUID: libvirtutils.UUIDStringToBytes(machineID),
	}
}
