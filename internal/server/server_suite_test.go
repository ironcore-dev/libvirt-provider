// SPDX-FileCopyrightText: 20253 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package server_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ironcore-dev/ironcore-image/oci/remote"
	ocistore "github.com/ironcore-dev/ironcore-image/oci/store"
	corev1alpha1 "github.com/ironcore-dev/ironcore/api/core/v1alpha1"
	iriv1alpha1 "github.com/ironcore-dev/ironcore/iri/apis/machine/v1alpha1"
	"github.com/ironcore-dev/ironcore/iri/remote/machine"
	"github.com/ironcore-dev/libvirt-provider/api"
	"github.com/ironcore-dev/libvirt-provider/cmd/libvirt-provider/app"
	"github.com/ironcore-dev/libvirt-provider/internal/host"
	libvirtutils "github.com/ironcore-dev/libvirt-provider/internal/libvirt/utils"
	"github.com/ironcore-dev/libvirt-provider/internal/mcr"
	"github.com/ironcore-dev/libvirt-provider/internal/networkinterfaceplugin"
	volumeplugin "github.com/ironcore-dev/libvirt-provider/internal/plugins/volume"
	"github.com/ironcore-dev/libvirt-provider/internal/plugins/volume/localdisk"
	"github.com/ironcore-dev/libvirt-provider/internal/raw"
	"github.com/ironcore-dev/libvirt-provider/internal/server"
	"github.com/ironcore-dev/libvirt-provider/internal/strategy"
	claim "github.com/ironcore-dev/provider-utils/claimutils/claim"
	"github.com/ironcore-dev/provider-utils/claimutils/gpu"
	"github.com/ironcore-dev/provider-utils/claimutils/pci"
	"github.com/ironcore-dev/provider-utils/eventutils/recorder"
	ocihostutils "github.com/ironcore-dev/provider-utils/ociutils/host"
	ociutils "github.com/ironcore-dev/provider-utils/ociutils/oci"
	hostutils "github.com/ironcore-dev/provider-utils/storeutils/host"
	"github.com/ironcore-dev/provider-utils/storeutils/store"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

const (
	eventuallyTimeout              = 80 * time.Second
	pollingInterval                = 50 * time.Millisecond
	gracefulShutdownTimeout        = 60 * time.Second
	resyncGarbageCollectorInterval = 5 * time.Second
	resyncVolumeSizeInterval       = 1 * time.Minute
	consistentlyDuration           = 1 * time.Second
	probeEveryInterval             = 2 * time.Second
	machineClassx3xlarge           = "x3-xlarge"
	machineClassx2medium           = "x2-medium"
	machineClassx2mediumgpu        = "x2-medium-gpu"
	osImage                        = "ghcr.io/ironcore-dev/os-images/virtualization/gardenlinux:latest"
	emptyDiskSize                  = 1024 * 1024 * 1024
	baseURL                        = "http://localhost:20251"
	streamingAddress               = "127.0.0.1:20251"
	healthCheckAddress             = "127.0.0.1:20252"
	metricsAddress                 = "" // disable metrics server for integration tests
	machineEventMaxEvents          = 1000
	machineEventTTL                = 10 * time.Second
	machineEventResyncInterval     = 2 * time.Second
)

var (
	machineClient  iriv1alpha1.MachineRuntimeClient
	machineClasses *mcr.Mcr
	machineStore   *hostutils.Store[*api.Machine]
	resClaimer     claim.Claimer
	tempDir        string
)

func TestServer(t *testing.T) {
	SetDefaultConsistentlyPollingInterval(pollingInterval)
	SetDefaultEventuallyPollingInterval(pollingInterval)
	SetDefaultEventuallyTimeout(eventuallyTimeout)
	SetDefaultConsistentlyDuration(consistentlyDuration)

	RegisterFailHandler(Fail)
	RunSpecs(t, "GRPC Server Suite", Label("integration"))
}

var _ = BeforeSuite(func(ctx SpecContext) {
	log := zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true))
	logf.SetLogger(log)

	By("starting the app")

	pluginOpts := networkinterfaceplugin.NewDefaultOptions()
	pluginOpts.PluginName = "isolated"

	tempDir = GinkgoT().TempDir()
	Expect(os.Chmod(tempDir, 0730)).Should(Succeed())

	rootDir := filepath.Join(tempDir, "libvirt-provider")

	address := filepath.Join(tempDir, "test.sock")

	By("setting up libvirt connection")
	libvirtOpts := app.LibvirtOptions{
		PreferredDomainTypes:  []string{"kvm", "qemu"},
		PreferredMachineTypes: []string{"pc-q35", "pc-i440fx", "virt"},
		Qcow2Type:             "exec",
	}
	libvirt, err := libvirtutils.GetLibvirt(libvirtOpts.Socket, libvirtOpts.Address, libvirtOpts.URI)
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(libvirt.ConnectClose)

	By("setting up the machine store")
	providerHost, err := host.NewLibvirtAt(rootDir, libvirt)
	Expect(err).NotTo(HaveOccurred())

	By("setting up the machine store")
	machineStore, err = hostutils.NewStore[*api.Machine](hostutils.Options[*api.Machine]{
		NewFunc:        func() *api.Machine { return &api.Machine{} },
		CreateStrategy: strategy.MachineStrategy,
		Dir:            providerHost.MachineStoreDir(),
	})
	Expect(err).NotTo(HaveOccurred())

	By("setting up the machine class registry")
	classes := []*iriv1alpha1.MachineClass{
		{
			Name: machineClassx3xlarge,
			Capabilities: &iriv1alpha1.MachineClassCapabilities{
				Resources: map[string]int64{
					string(corev1alpha1.ResourceCPU):    4,
					string(corev1alpha1.ResourceMemory): 8589934592,
				},
			},
		},
		{
			Name: machineClassx2medium,
			Capabilities: &iriv1alpha1.MachineClassCapabilities{
				Resources: map[string]int64{
					string(corev1alpha1.ResourceCPU):    2,
					string(corev1alpha1.ResourceMemory): 2147483648,
				},
			},
		},
		{
			Name: machineClassx2mediumgpu,
			Capabilities: &iriv1alpha1.MachineClassCapabilities{
				Resources: map[string]int64{
					string(corev1alpha1.ResourceCPU):    2,
					string(corev1alpha1.ResourceMemory): 2147483648,
					api.NvidiaGPUPlugin:                 2,
				},
			},
		},
	}
	machineClasses, err = mcr.NewMachineClassRegistry(classes)
	Expect(err).NotTo(HaveOccurred())

	By("setting up the event store")
	eventStore := recorder.NewEventStore(log, recorder.EventStoreOptions{
		MaxEvents:      machineEventMaxEvents,
		TTL:            machineEventTTL,
		ResyncInterval: machineEventResyncInterval,
	})

	By("setting up the volume plugin")
	platform, err := ocihostutils.Platform()
	Expect(err).NotTo(HaveOccurred())

	reg, err := remote.DockerRegistryWithPlatform(nil, platform)
	Expect(err).NotTo(HaveOccurred())

	ociStore, err := ocistore.New(providerHost.ImagesDir())
	Expect(err).NotTo(HaveOccurred())

	imgCache, err := ociutils.NewLocalCache(log, reg, ociStore, nil)
	Expect(err).NotTo(HaveOccurred())

	rawInst, err := raw.Instance(raw.Default())
	Expect(err).NotTo(HaveOccurred())

	volumePlugins := volumeplugin.NewPluginManager()
	err = volumePlugins.InitPlugins(providerHost, []volumeplugin.Plugin{
		// ceph.NewPlugin(),
		localdisk.NewPlugin(rawInst, imgCache),
	})
	Expect(err).NotTo(HaveOccurred())

	By("setting up the network interface plugin")
	nicPlugin, _, _ := pluginOpts.NetworkInterfacePlugin()

	resClaimer, err = claim.NewResourceClaimer(
		log, gpu.NewGPUClaimPlugin(log, api.NvidiaGPUPlugin, NewTestingPCIReader([]pci.Address{
			{Domain: 0, Bus: 3, Slot: 0, Function: 0},
			{Domain: 0, Bus: 3, Slot: 0, Function: 1},
		}), []pci.Address{}),
	)
	Expect(err).ToNot(HaveOccurred())

	srv, err := server.New(server.Options{
		BaseURL:         baseURL,
		Libvirt:         libvirt,
		MachineStore:    machineStore,
		EventStore:      eventStore,
		MachineClasses:  machineClasses,
		VolumePlugins:   volumePlugins,
		NetworkPlugins:  nicPlugin,
		ResourceClaimer: resClaimer,
		EnableHugepages: false,
		GuestAgent:      api.GuestAgentNone,
	})
	Expect(err).NotTo(HaveOccurred())

	cancelCtx, cancel := context.WithCancel(context.Background())
	DeferCleanup(cancel)

	go func() {
		defer GinkgoRecover()
		Expect(app.RunGRPCServer(cancelCtx, log, log, srv, address)).To(Succeed())
	}()

	go func() {
		defer GinkgoRecover()
		Expect(resClaimer.Start(cancelCtx)).To(Succeed())
	}()

	Expect(resClaimer.WaitUntilStarted(ctx)).To(Succeed())

	Eventually(func() error {
		return isSocketAvailable(address)
	}).WithTimeout(30 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())

	address, err = machine.GetAddressWithTimeout(3*time.Second, fmt.Sprintf("unix://%s", address))
	Expect(err).NotTo(HaveOccurred())

	gconn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(gconn.Close)

	machineClient = iriv1alpha1.NewMachineRuntimeClient(gconn)
})

func isSocketAvailable(socketPath string) error {
	fileInfo, err := os.Stat(socketPath)
	if err != nil {
		return err
	}
	if fileInfo.Mode()&os.ModeSocket != 0 {
		return nil
	}
	return fmt.Errorf("socket %s is not available", socketPath)
}

func cleanupMachine(machineID string) func(SpecContext) {
	return func(ctx SpecContext) {
		By(fmt.Sprintf("Cleaning up machine ID=%s", machineID))

		Eventually(func(g Gomega) error {
			// Get machine to release claims
			m, err := machineStore.Get(ctx, machineID)
			if errors.Is(err, store.ErrNotFound) {
				return nil
			}

			if err != nil {
				GinkgoWriter.Printf("Getting machine failed ID=%s: err=%v\n", machineID, err)
				return err
			}

			// Release GPU claims
			if len(m.Spec.Gpu) > 0 {
				GinkgoWriter.Printf("Releasing claims ID=%s: claims=%s\n", machineID, m.Spec.Gpu)
				claimer := resClaimer
				err = claimer.Release(ctx, claim.Claims{
					api.NvidiaGPUPlugin: gpu.NewGPUClaim(m.Spec.Gpu),
				})
				if err != nil {
					GinkgoWriter.Printf("Releasing claims failed ID=%s: claims=%v, err=%v\n", machineID, m.Spec.Gpu, err)
					return err
				}
			}

			err = machineStore.Delete(ctx, machineID)
			GinkgoWriter.Printf("Deleting machine ID=%s: err=%v\n", machineID, err)
			if errors.Is(err, store.ErrNotFound) {
				return nil
			}
			return err
		}).Should(Succeed())
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
