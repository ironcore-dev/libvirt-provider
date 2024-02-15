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
	cephproviderapp "github.com/ironcore-dev/ceph-provider/iri/volume/cmd/volume/app"
	machineiriv1alpha1 "github.com/ironcore-dev/ironcore/iri/apis/machine/v1alpha1"
	"github.com/ironcore-dev/ironcore/iri/remote/machine"
	"github.com/ironcore-dev/ironcore/iri/remote/volume"
	"github.com/ironcore-dev/libvirt-provider/provider/cmd/app"
	"github.com/ironcore-dev/libvirt-provider/provider/networkinterfaceplugin"
	. "github.com/onsi/ginkgo/v2"

	volumeiriv1alpha1 "github.com/ironcore-dev/ironcore/iri/apis/volume/v1alpha1"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

const (
	eventuallyTimeout    = 30 * time.Second
	pollingInterval      = 50 * time.Millisecond
	consistentlyDuration = 1 * time.Second
	machineClassx3xlarge = "x3-xlarge"
	machineClassx2medium = "x2-medium"
	baseURL              = "http://localhost:20251"
	streamingAddress     = "127.0.0.1:20251"
)

var (
	machineClient       machineiriv1alpha1.MachineRuntimeClient
	volumeClient        volumeiriv1alpha1.VolumeRuntimeClient
	libvirtConn         *libvirt.Libvirt
	machineClassesFile  *os.File
	tempDir             string
	cephMonitors        = os.Getenv("CEPH_MONITORS")
	cephImage           = os.Getenv("CEPH_IMAGE")
	cephUsername        = os.Getenv("CEPH_USERNAME")
	cephUserkey         = os.Getenv("CEPH_USERKEY")
	cephKeyringFilename = os.Getenv("CEPH_KEYRING_FILENAME")
	cephPoolname        = os.Getenv("CEPH_POOLNAME")
	cephClientname      = os.Getenv("CEPH_CLIENTNAME")
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

	By("Running libvirt-provider app")

	machineClasses := []machineiriv1alpha1.MachineClass{
		{
			Name: machineClassx3xlarge,
			Capabilities: &machineiriv1alpha1.MachineClassCapabilities{
				CpuMillis:   4000,
				MemoryBytes: 8589934592,
			},
		},
		{
			Name: machineClassx2medium,
			Capabilities: &machineiriv1alpha1.MachineClassCapabilities{
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

	tempDir = GinkgoT().TempDir()
	Expect(os.Chmod(tempDir, 0730)).Should(Succeed())

	opts := app.Options{
		Address:                     filepath.Join(tempDir, "test.sock"),
		BaseURL:                     baseURL,
		PathSupportedMachineClasses: machineClassesFile.Name(),
		RootDir:                     filepath.Join(tempDir, "libvirt-provider"),
		StreamingAddress:            streamingAddress,
		Libvirt: app.LibvirtOptions{
			Socket:                "/var/run/libvirt/libvirt-sock",
			URI:                   "qemu:///system",
			PreferredDomainTypes:  []string{"kvm", "qemu"},
			PreferredMachineTypes: []string{"pc-q35", "pc-i440fx"},
			Qcow2Type:             "exec",
		},
		NicPlugin:                      pluginOpts,
		GCVMGracefulShutdownTimeout:    10 * time.Second,
		ResyncIntervalGarbageCollector: 5 * time.Second,
		ResyncIntervalVolumeSize:       1 * time.Minute,
		VirshExecutable:                "virsh",
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

	gconn, err := grpc.Dial(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(gconn.Close)

	machineClient = machineiriv1alpha1.NewMachineRuntimeClient(gconn)

	c := dialers.NewLocal()
	libvirtConn = libvirt.NewWithDialer(c)
	Expect(libvirtConn.Connect()).To(Succeed())
	Expect(libvirtConn.IsConnected(), BeTrue())
	DeferCleanup(libvirtConn.ConnectClose)

	By("Running ceph-provider app for ceph volume creation")
	keyEncryptionKeyFile, err := os.CreateTemp(GinkgoT().TempDir(), "keyencryption")
	Expect(err).NotTo(HaveOccurred())
	defer func() {
		_ = keyEncryptionKeyFile.Close()
	}()
	Expect(os.WriteFile(keyEncryptionKeyFile.Name(), []byte("abcjdkekakakakakakakkadfkkasfdks"), 0666)).To(Succeed()) //TODO: try for minimal permissions

	volumeClasses := []volumeiriv1alpha1.VolumeClass{{
		Name: "foo",
		Capabilities: &volumeiriv1alpha1.VolumeClassCapabilities{
			Tps:  100,
			Iops: 100,
		},
	}}
	volumeClassesData, err := json.Marshal(volumeClasses)
	Expect(err).NotTo(HaveOccurred())

	volumeClassesFile, err := os.CreateTemp(GinkgoT().TempDir(), "volumeclasses")
	Expect(err).NotTo(HaveOccurred())
	defer func() {
		_ = volumeClassesFile.Close()
	}()
	Expect(os.WriteFile(volumeClassesFile.Name(), volumeClassesData, 0666)).To(Succeed()) //TODO: try for minimal permissions

	cephProviderOps := cephproviderapp.Options{
		Address:                    fmt.Sprintf("%s/ceph-volume-provider.sock", os.Getenv("PWD")),
		PathSupportedVolumeClasses: volumeClassesFile.Name(),
		Ceph: cephproviderapp.CephOptions{
			ConnectTimeout:         10 * time.Second,
			Monitors:               cephMonitors,
			User:                   cephUsername,
			KeyringFile:            cephKeyringFilename,
			Pool:                   cephPoolname,
			Client:                 cephClientname,
			KeyEncryptionKeyPath:   keyEncryptionKeyFile.Name(),
			BurstDurationInSeconds: 15,
		},
	}

	srvCtxCeph, cancel := context.WithCancel(context.Background())
	DeferCleanup(cancel)

	go func() {
		defer GinkgoRecover()
		Expect(cephproviderapp.Run(srvCtxCeph, cephProviderOps)).To(Succeed())
	}()

	Eventually(func() error {
		return isSocketAvailable(cephProviderOps.Address)
	}).WithTimeout(30 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())

	volumeAddress, err := volume.GetAddressWithTimeout(3*time.Second, fmt.Sprintf("unix://%s", cephProviderOps.Address))
	Expect(err).NotTo(HaveOccurred())

	gconnVolume, err := grpc.Dial(volumeAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	Expect(err).NotTo(HaveOccurred())

	volumeClient = volumeiriv1alpha1.NewVolumeRuntimeClient(gconnVolume)
	DeferCleanup(gconnVolume.Close)
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
