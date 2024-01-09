// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package server_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/digitalocean/go-libvirt"
	"github.com/digitalocean/go-libvirt/socket/dialers"
	iriv1alpha1 "github.com/ironcore-dev/ironcore/iri/apis/machine/v1alpha1"
	"github.com/ironcore-dev/ironcore/iri/remote/machine"
	"github.com/ironcore-dev/libvirt-provider/provider/cmd/app"
	"github.com/ironcore-dev/libvirt-provider/provider/networkinterfaceplugin"
	. "github.com/onsi/ginkgo/v2"

	. "github.com/onsi/gomega"
	"github.com/spf13/pflag"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var (
	machineClient iriv1alpha1.MachineRuntimeClient
	libvirtConn   *libvirt.Libvirt
)

const (
	eventuallyTimeout    = 180 * time.Second
	pollingInterval      = 50 * time.Millisecond
	consistentlyDuration = 1 * time.Second
	machineClassx3xlarge = "x3-xlarge"
	baseURL              = "http://localhost:8080"
	streamingAddress     = "127.0.0.1:20251"
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
	logf.SetLogger(GinkgoLogr)

	machineClasses := []iriv1alpha1.MachineClass{
		{
			Name: machineClassx3xlarge,
			Capabilities: &iriv1alpha1.MachineClassCapabilities{
				CpuMillis:   4000,
				MemoryBytes: 8589934592,
			},
		},
	}
	machineClassData, err := json.Marshal(machineClasses)
	Expect(err).NotTo(HaveOccurred())

	machineClassesFile, err := os.CreateTemp(GinkgoT().TempDir(), "machineclasses")
	Expect(err).NotTo(HaveOccurred())
	defer func() {
		_ = machineClassesFile.Close()
	}()
	Expect(os.WriteFile(machineClassesFile.Name(), machineClassData, 0600)).To(Succeed())

	By("starting the app")

	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	apinetPlugin := networkinterfaceplugin.NewDefaultOptions()
	apinetPlugin.PluginName = "apinet"
	apinetPlugin.AddFlags(fs)
	Expect(fs.Set("apinet-node-name", "test-node")).To(Succeed())

	tempDir := GinkgoT().TempDir()
	Expect(os.Chmod(tempDir, 0730)).Should(Succeed())

	opts := app.Options{
		Address:                     fmt.Sprintf("%s/test.sock", os.Getenv("PWD")),
		BaseURL:                     baseURL,
		PathSupportedMachineClasses: machineClassesFile.Name(),
		RootDir:                     fmt.Sprintf("%s/libvirt-provider", tempDir),
		StreamingAddress:            streamingAddress,
		Libvirt: app.LibvirtOptions{
			Socket:                "/var/run/libvirt/libvirt-sock",
			URI:                   "qemu:///system",
			PreferredDomainTypes:  []string{"kvm", "qemu"},
			PreferredMachineTypes: []string{"pc-q35", "pc-i440fx-2.9", "pc-i440fx-2.8"},
			Qcow2Type:             "exec",
		},
		NicPlugin: apinetPlugin,
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
