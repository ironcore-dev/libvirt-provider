// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package server_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/digitalocean/go-libvirt"
	"github.com/digitalocean/go-libvirt/socket/dialers"
	iriv1alpha1 "github.com/ironcore-dev/ironcore/iri/apis/machine/v1alpha1"
	"github.com/ironcore-dev/ironcore/iri/remote/machine"
	"github.com/ironcore-dev/libvirt-provider/api"
	"github.com/ironcore-dev/libvirt-provider/cmd/libvirt-provider/app"
	"github.com/ironcore-dev/libvirt-provider/internal/event/machineevent"
	libvirtutils "github.com/ironcore-dev/libvirt-provider/internal/libvirt/utils"
	"github.com/ironcore-dev/libvirt-provider/internal/networkinterfaceplugin"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

const (
	eventuallyTimeout    = 80 * time.Second
	pollingInterval      = 50 * time.Millisecond
	consistentlyDuration = 1 * time.Second
	probeEveryInterval   = 2 * time.Second

	squashfsOSImage = "ghcr.io/ironcore-dev/ironcore-image/gardenlinux:squashfs-dev-20240123-v2"
	emptyDiskSize   = 1024 * 1024 * 1024

	//Test graceful shutdown will be tested separately
	resyncGarbageCollectorInterval = 5 * time.Second
	resyncVolumeSizeInterval       = 1 * time.Minute
	baseURL                        = "http://localhost:20251"
	streamingAddress               = "127.0.0.1:20251"
	healthCheckAddress             = "127.0.0.1:20252"
	metricsAddress                 = "" // disable metrics server for integration tests

	machineClassx3xlarge = "x3-xlarge"
	machineClassx2medium = "x2-medium"
)

var (
	machineClient      iriv1alpha1.MachineRuntimeClient
	libvirtConn        *libvirt.Libvirt
	machineClassesFile *os.File
	tempDir            string
)

func TestServer(t *testing.T) {
	SetDefaultConsistentlyPollingInterval(pollingInterval)
	SetDefaultEventuallyPollingInterval(pollingInterval)
	SetDefaultEventuallyTimeout(eventuallyTimeout)
	SetDefaultConsistentlyDuration(consistentlyDuration)

	RegisterFailHandler(Fail)
	RunSpecs(t, "GRPC Server Suite", Label("integration"))
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	By("starting the app")

	machineClasses := []iriv1alpha1.MachineClass{
		{
			Name: machineClassx3xlarge,
			Capabilities: &iriv1alpha1.MachineClassCapabilities{
				CpuMillis:   4000,
				MemoryBytes: 8589934592,
			},
		},
		{
			Name: machineClassx2medium,
			Capabilities: &iriv1alpha1.MachineClassCapabilities{
				CpuMillis:   2000,
				MemoryBytes: 2147483648,
			},
		},
	}
	machineClassData, err := json.Marshal(machineClasses)
	Expect(err).NotTo(HaveOccurred())
	machineClassesFile, err = os.CreateTemp(GinkgoT().TempDir(), "machineclasses")
	Expect(err).NotTo(HaveOccurred())
	Expect(os.WriteFile(machineClassesFile.Name(), machineClassData, 0600)).To(Succeed())
	DeferCleanup(machineClassesFile.Close)
	DeferCleanup(os.Remove, machineClassesFile.Name())

	pluginOpts := networkinterfaceplugin.NewDefaultOptions()
	pluginOpts.PluginName = "isolated"

	// tempDir = "/home/pcmil/libvirt-provider/temptest"
	tempDir = GinkgoT().TempDir()
	Expect(os.Chmod(tempDir, 0730)).Should(Succeed())

	opts := app.Options{
		Address:                     filepath.Join(tempDir, "test.sock"),
		BaseURL:                     baseURL,
		PathSupportedMachineClasses: machineClassesFile.Name(),
		RootDir:                     filepath.Join(tempDir, "libvirt-provider"),
		StreamingAddress:            streamingAddress,
		Servers: app.ServersOptions{
			Metrics: app.HTTPServerOptions{
				Addr: metricsAddress,
			},
			HealthCheck: app.HTTPServerOptions{
				Addr: healthCheckAddress,
			},
		},
		Libvirt: app.LibvirtOptions{
			Socket:                "/var/run/libvirt/libvirt-sock",
			URI:                   "qemu:///system",
			PreferredDomainTypes:  []string{"kvm", "qemu"},
			PreferredMachineTypes: []string{"pc-q35", "pc-i440fx"},
			Qcow2Type:             "exec",
		},
		NicPlugin: pluginOpts,
		// GCVMGracefulShutdownTimeout:    gracefulShutdownTimeout,
		ResyncIntervalGarbageCollector: resyncGarbageCollectorInterval,
		ResyncIntervalVolumeSize:       resyncVolumeSizeInterval,
		GuestAgent:                     app.GuestAgentOption(api.GuestAgentNone),
		MachineEventStore: machineevent.EventStoreOptions{
			MachineEventMaxEvents:      5,
			MachineEventTTL:            10 * time.Second,
			MachineEventResyncInterval: 10 * time.Second,
		},
	}

	srvCtx, cancel := context.WithCancel(context.Background())
	DeferCleanup(cancel)

	go func() {
		defer GinkgoRecover()
		Expect(app.Run(srvCtx, opts)).To(Succeed())
	}()

	Eventually(func() error {
		return isSocketAvailable(opts.Address)
	}).WithTimeout(30 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())

	address, err := machine.GetAddressWithTimeout(3*time.Second, fmt.Sprintf("unix://%s", opts.Address))
	Expect(err).NotTo(HaveOccurred())

	gconn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(gconn.Close)

	machineClient = iriv1alpha1.NewMachineRuntimeClient(gconn)

	c := dialers.NewLocal()
	libvirtConn = libvirt.NewWithDialer(c)
	Expect(libvirtConn.Connect()).To(Succeed())
	Expect(libvirtConn.IsConnected(), BeTrue())
	DeferCleanup(libvirtConn.ConnectClose)
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

func assertMachineIsRunning(machineID string) libvirt.Domain {
	GinkgoHelper()
	By("ensuring domain and domain XML is created for machine")
	var domain libvirt.Domain

	Eventually(func() (err error) {
		domain, err = libvirtConn.DomainLookupByUUID(libvirtutils.UUIDStringToBytes(machineID))
		return err
	}).Should(Succeed())

	domainXMLData, err := libvirtConn.DomainGetXMLDesc(domain, 0)
	Expect(err).NotTo(HaveOccurred())
	Expect(domainXMLData).NotTo(BeEmpty())

	By("ensuring domain for machine is in running state")
	Eventually(func(g Gomega) libvirt.DomainState {
		domainState, _, err := libvirtConn.DomainGetState(domain, 0)
		g.Expect(err).NotTo(HaveOccurred())
		return libvirt.DomainState(domainState)
	}).Should(Equal(libvirt.DomainRunning))

	return domain
}
