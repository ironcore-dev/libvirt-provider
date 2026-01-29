// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controllers_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ironcore-dev/provider-utils/claimutils/claim"
	"github.com/ironcore-dev/provider-utils/claimutils/gpu"

	"github.com/digitalocean/go-libvirt"
	"github.com/google/uuid"
	"github.com/ironcore-dev/ironcore-image/oci/remote"
	ocistore "github.com/ironcore-dev/ironcore-image/oci/store"
	"github.com/ironcore-dev/libvirt-provider/api"
	"github.com/ironcore-dev/libvirt-provider/cmd/libvirt-provider/app"
	"github.com/ironcore-dev/libvirt-provider/internal/controllers"
	"github.com/ironcore-dev/libvirt-provider/internal/host"
	"github.com/ironcore-dev/libvirt-provider/internal/libvirt/guest"
	libvirtutils "github.com/ironcore-dev/libvirt-provider/internal/libvirt/utils"
	"github.com/ironcore-dev/libvirt-provider/internal/networkinterfaceplugin"
	providernetworkinterface "github.com/ironcore-dev/libvirt-provider/internal/plugins/networkinterface"
	volumeplugin "github.com/ironcore-dev/libvirt-provider/internal/plugins/volume"
	"github.com/ironcore-dev/libvirt-provider/internal/plugins/volume/localdisk"
	"github.com/ironcore-dev/libvirt-provider/internal/raw"
	"github.com/ironcore-dev/libvirt-provider/internal/strategy"
	apiutils "github.com/ironcore-dev/provider-utils/apiutils/api"
	"github.com/ironcore-dev/provider-utils/claimutils/pci"
	"github.com/ironcore-dev/provider-utils/eventutils/event"
	"github.com/ironcore-dev/provider-utils/eventutils/recorder"
	ocihostutils "github.com/ironcore-dev/provider-utils/ociutils/host"
	ociutils "github.com/ironcore-dev/provider-utils/ociutils/oci"
	hostutils "github.com/ironcore-dev/provider-utils/storeutils/host"
	"github.com/ironcore-dev/provider-utils/storeutils/store"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

const (
	eventuallyTimeout              = 80 * time.Second
	pollingInterval                = 50 * time.Millisecond
	gracefulShutdownTimeout        = 0 * time.Second
	resyncGarbageCollectorInterval = 5 * time.Second
	consistentlyDuration           = 1 * time.Second
	machineClassx3xlarge           = "x3-xlarge"
	machineClassx2medium           = "x2-medium"
	osImage                        = "ghcr.io/ironcore-dev/os-images/virtualization/gardenlinux:latest"
	emptyDiskSize                  = 1024 * 1024 * 1024
	machineEventMaxEvents          = 1000
	machineEventTTL                = 10 * time.Second
	machineEventResyncInterval     = 2 * time.Second
)

var (
	machineController *controllers.MachineReconciler
	machineStore      *hostutils.Store[*api.Machine]
	machineEvents     *event.ListWatchSource[*api.Machine]
	eventRecorder     recorder.EventRecorder
	volumePlugins     *volumeplugin.PluginManager
	networkPlugin     providernetworkinterface.Plugin
	providerHost      host.LibvirtHost
	libvirtConn       *libvirt.Libvirt
	tempDir           string
	controllerCtx     context.Context
	controllerCancel  context.CancelFunc
	resClaimer        claim.Claimer
)

func TestControllers(t *testing.T) {
	SetDefaultConsistentlyPollingInterval(pollingInterval)
	SetDefaultEventuallyPollingInterval(pollingInterval)
	SetDefaultEventuallyTimeout(eventuallyTimeout)
	SetDefaultConsistentlyDuration(consistentlyDuration)

	RegisterFailHandler(Fail)
	RunSpecs(t, "Machine Controller Suite", Label("integration"))
}

var _ = BeforeSuite(func() {
	log := zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true))
	logf.SetLogger(log)

	By("setting up test environment")

	// Use a very short temp directory path to avoid Unix socket path length limits (108 chars)
	// Socket paths can be like: /tmp/t123/machines/uuid/qemu-guest-agent.sock (~85 chars)
	tempDir = filepath.Join("/tmp", fmt.Sprintf("t%d%v", time.Now().Unix(), GinkgoParallelProcess()))
	Expect(os.MkdirAll(tempDir, 0730)).Should(Succeed())
	DeferCleanup(func() {
		_ = os.RemoveAll(tempDir)
	})

	rootDir := tempDir

	By("setting up libvirt connection")
	libvirtOpts := app.LibvirtOptions{
		PreferredDomainTypes:  []string{"kvm", "qemu"},
		PreferredMachineTypes: []string{"pc-q35", "pc-i440fx", "virt"},
		Qcow2Type:             "exec",
	}
	libvirt, err := libvirtutils.GetLibvirt(libvirtOpts.Socket, libvirtOpts.Address, libvirtOpts.URI)
	Expect(err).NotTo(HaveOccurred())
	libvirtConn = libvirt
	DeferCleanup(libvirt.ConnectClose)

	By("setting up provider host")
	providerHost, err = host.NewLibvirtAt(rootDir, libvirt)
	Expect(err).NotTo(HaveOccurred())

	By("setting up machine store")
	machineStore, err = hostutils.NewStore[*api.Machine](hostutils.Options[*api.Machine]{
		NewFunc:        func() *api.Machine { return &api.Machine{} },
		CreateStrategy: strategy.MachineStrategy,
		Dir:            providerHost.MachineStoreDir(),
	})
	Expect(err).NotTo(HaveOccurred())

	By("setting up machine events")
	machineEvents, err = event.NewListWatchSource[*api.Machine](
		machineStore.List,
		machineStore.Watch,
		event.ListWatchSourceOptions{
			ResyncDuration: 30 * time.Second,
		},
	)
	Expect(err).NotTo(HaveOccurred())

	By("setting up event store")
	eventRecorder = recorder.NewEventStore(log, recorder.EventStoreOptions{
		MaxEvents:      machineEventMaxEvents,
		TTL:            machineEventTTL,
		ResyncInterval: machineEventResyncInterval,
	})

	By("detecting guest capabilities")
	caps, err := guest.DetectCapabilities(libvirt, guest.CapabilitiesOptions{
		PreferredDomainTypes:  libvirtOpts.PreferredDomainTypes,
		PreferredMachineTypes: libvirtOpts.PreferredMachineTypes,
	})
	Expect(err).NotTo(HaveOccurred())
	GinkgoWriter.Printf("Detected guest capabilities\n")

	By("setting up OCI registry and image cache")
	platform, err := ocihostutils.Platform()
	Expect(err).NotTo(HaveOccurred())
	GinkgoWriter.Printf("Platform: %s\n", platform.Architecture)

	reg, err := remote.DockerRegistryWithPlatform(nil, platform)
	Expect(err).NotTo(HaveOccurred())

	ociStore, err := ocistore.New(providerHost.ImagesDir())
	Expect(err).NotTo(HaveOccurred())

	imgCache, err := ociutils.NewLocalCache(log, reg, ociStore, nil)
	Expect(err).NotTo(HaveOccurred())

	By("setting up raw instance")
	rawInst, err := raw.Instance(raw.Default())
	Expect(err).NotTo(HaveOccurred())

	By("setting up volume plugins")
	volumePlugins = volumeplugin.NewPluginManager()
	err = volumePlugins.InitPlugins(providerHost, []volumeplugin.Plugin{
		localdisk.NewPlugin(rawInst, imgCache),
	})
	Expect(err).NotTo(HaveOccurred())

	By("setting up network interface plugin")
	pluginOpts := networkinterfaceplugin.NewDefaultOptions()
	pluginOpts.PluginName = "isolated"
	var cleanup func()
	networkPlugin, cleanup, err = pluginOpts.NetworkInterfacePlugin()
	Expect(err).NotTo(HaveOccurred())
	if cleanup != nil {
		DeferCleanup(cleanup)
	}
	Expect(networkPlugin.Init(providerHost)).To(Succeed())

	By("setting up resource claimer")
	resClaimer, err = claim.NewResourceClaimer(
		log, gpu.NewGPUClaimPlugin(log, api.NvidiaGPUPlugin, NewTestingPCIReader([]pci.Address{}), []pci.Address{}),
	)
	Expect(err).ToNot(HaveOccurred())

	By("creating machine controller")
	machineController, err = controllers.NewMachineReconciler(
		log.WithName("machine-controller"),
		providerHost,
		machineStore,
		machineEvents,
		eventRecorder,
		controllers.MachineReconcilerOptions{
			GuestCapabilities:              caps,
			ImageCache:                     imgCache,
			Raw:                            rawInst,
			VolumePluginManager:            volumePlugins,
			NetworkInterfacePlugin:         networkPlugin,
			ResourceClaimer:                resClaimer,
			ResyncIntervalGarbageCollector: resyncGarbageCollectorInterval,
			EnableHugepages:                false,
			GCVMGracefulShutdownTimeout:    gracefulShutdownTimeout,
			VolumeCachePolicy:              "writethrough",
		},
	)
	Expect(err).NotTo(HaveOccurred())

	By("starting machine events")
	controllerCtx, controllerCancel = context.WithCancel(context.Background())
	DeferCleanup(controllerCancel)

	go func() {
		defer GinkgoRecover()
		err := machineEvents.Start(controllerCtx)
		if err != nil && controllerCtx.Err() == nil {
			// Only fail if not cancelled
			Expect(err).NotTo(HaveOccurred())
		}
	}()

	By("starting machine controller")
	go func() {
		defer GinkgoRecover()
		err := machineController.Start(controllerCtx)
		if err != nil && controllerCtx.Err() == nil {
			// Only fail if not cancelled
			Expect(err).NotTo(HaveOccurred())
		}
	}()

	By("starting image cache")
	go func() {
		defer GinkgoRecover()
		err := imgCache.Start(controllerCtx)
		if err != nil && controllerCtx.Err() == nil {
			// Only fail if not cancelled
			Expect(err).NotTo(HaveOccurred())
		}
	}()

	// Wait a bit for controller and events to start
	time.Sleep(500 * time.Millisecond)

	By("controller setup complete")
})

func createMachine(spec api.MachineSpec) (*api.Machine, error) {
	machine := &api.Machine{
		Metadata: apiutils.Metadata{
			ID: uuid.NewString(),
		},
		Spec: spec,
	}
	return machineStore.Create(context.Background(), machine)
}

func getMachine(id string) (*api.Machine, error) {
	return machineStore.Get(context.Background(), id)
}

func deleteMachine(id string) error {
	return machineStore.Delete(context.Background(), id)
}

func updateMachine(machine *api.Machine) (*api.Machine, error) {
	m, err := getMachine(machine.ID)
	if err != nil {
		return nil, err
	}
	GinkgoWriter.Printf("ResourceVersion: ID=%s\n", m.ResourceVersion)
	machine.ResourceVersion = m.ResourceVersion
	return machineStore.Update(context.Background(), machine)
}

func cleanupMachine(machineID string) func(SpecContext) {
	return func(ctx SpecContext) {
		By(fmt.Sprintf("Cleaning up machine ID=%s", machineID))
		Eventually(func(g Gomega) error {
			err := deleteMachine(machineID)
			GinkgoWriter.Printf("Deleting machine ID=%s: err=%v\n", machineID, err)
			if errors.Is(err, store.ErrNotFound) {
				return nil
			}
			return err
		}).Should(Succeed())

		Eventually(func(g Gomega) bool {
			_, err := libvirtConn.DomainLookupByUUID(libvirtutils.UUIDStringToBytes(machineID))
			if err != nil {
				GinkgoWriter.Printf("Checking if domain still exists for machine ID=%s: err=%v\n", machineID, err)
			}
			return libvirt.IsNotFound(err)
		}).WithPolling(time.Second).Should(BeTrue())
	}
}

type TestingPCIReader struct {
	pciAddrs []pci.Address
}

func (t TestingPCIReader) Read() ([]pci.Address, error) {
	return t.pciAddrs, nil
}

func NewTestingPCIReader(addrs []pci.Address) *TestingPCIReader {
	return &TestingPCIReader{
		pciAddrs: addrs,
	}
}
