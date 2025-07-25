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
	corev1alpha1 "github.com/ironcore-dev/ironcore/api/core/v1alpha1"
	iriv1alpha1 "github.com/ironcore-dev/ironcore/iri/apis/machine/v1alpha1"
	"github.com/ironcore-dev/ironcore/iri/remote/machine"
	"github.com/ironcore-dev/libvirt-provider/api"
	"github.com/ironcore-dev/libvirt-provider/cmd/libvirt-provider/app"
	"github.com/ironcore-dev/libvirt-provider/internal/networkinterfaceplugin"
	"github.com/ironcore-dev/provider-utils/eventutils/recorder"
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
	osImage                        = "ghcr.io/ironcore-dev/os-images/gardenlinux-amd64:latest"
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
	machineClient      iriv1alpha1.MachineRuntimeClient
	libvirtConn        *libvirt.Libvirt
	machineClassesFile *os.File
	tempDir            string
	cephMonitors       = os.Getenv("CEPH_MONITORS")
	cephImage          = os.Getenv("CEPH_IMAGE")
	cephUsername       = os.Getenv("CEPH_USERNAME")
	cephUserkey        = os.Getenv("CEPH_USERKEY")
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
		NicPlugin:                      pluginOpts,
		GCVMGracefulShutdownTimeout:    gracefulShutdownTimeout,
		ResyncIntervalGarbageCollector: resyncGarbageCollectorInterval,
		GuestAgent:                     app.GuestAgentOption(api.GuestAgentNone),
		MachineEventStore: recorder.EventStoreOptions{
			MaxEvents:      machineEventMaxEvents,
			TTL:            machineEventTTL,
			ResyncInterval: machineEventResyncInterval,
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
