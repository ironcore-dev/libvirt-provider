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

	iriv1alpha1 "github.com/ironcore-dev/ironcore/iri/apis/machine/v1alpha1"
	"github.com/ironcore-dev/ironcore/iri/remote/machine"
	"github.com/ironcore-dev/libvirt-provider/provider/cmd/app"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var (
	machineClient iriv1alpha1.MachineRuntimeClient
	testEnv       *envtest.Environment
	cfg           *rest.Config
)

const (
	eventuallyTimeout    = 3 * time.Second
	pollingInterval      = 50 * time.Millisecond
	consistentlyDuration = 1 * time.Second
	baseURL              = "http://localhost:8080"
	machineClassx3xlarge = "x3-xlarge"
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
	Expect(os.WriteFile(machineClassesFile.Name(), machineClassData, 0666)).To(Succeed())

	By("starting the app")
	user, err := testEnv.AddUser(envtest.User{
		Name:   "dummy",
		Groups: []string{"system:authenticated", "system:masters"},
	}, cfg)
	Expect(err).NotTo(HaveOccurred())
	kubeconfig, err := user.KubeConfig()
	Expect(err).NotTo(HaveOccurred())
	kubeConfigFile, err := os.CreateTemp(GinkgoT().TempDir(), "kubeconfig")
	Expect(err).NotTo(HaveOccurred())
	defer os.Remove(kubeConfigFile.Name())
	Expect(os.WriteFile(kubeConfigFile.Name(), kubeconfig, 0600)).To(Succeed())

	userHomeDir, err := os.UserHomeDir()
	Expect(err).NotTo(HaveOccurred())

	opts := app.Options{
		//TODO: set these using env variables if needed
		Address:                     fmt.Sprintf("%s/test.sock", os.Getenv("PWD")),
		BaseURL:                     baseURL,
		PathSupportedMachineClasses: machineClassesFile.Name(),
		RootDir:                     fmt.Sprintf("%s.libvirt-provider", userHomeDir),
		StreamingAddress:            streamingAddress,
		ApinetKubeconfig:            kubeConfigFile.Name(),
		// TODO: add other required fields
	}

	srvCtx, cancel := context.WithCancel(context.Background())
	DeferCleanup(cancel)

	go func() {
		defer GinkgoRecover()
		Expect(app.Run(srvCtx, opts)).To(Succeed())
	}()

	Eventually(func() (bool, error) {
		return isSocketAvailable(opts.Address)
	}, "30s", "500ms").Should(BeTrue(), "The UNIX socket file should be available")

	address, err := machine.GetAddressWithTimeout(3*time.Second, fmt.Sprintf("unix://%s", opts.Address))
	Expect(err).NotTo(HaveOccurred())

	gconn, err := grpc.Dial(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	Expect(err).NotTo(HaveOccurred())

	machineClient = iriv1alpha1.NewMachineRuntimeClient(gconn)
	DeferCleanup(gconn.Close)

	//TODO: setup libvirt client/connection to assert the results

})

func isSocketAvailable(socketPath string) (bool, error) {
	fileInfo, err := os.Stat(socketPath)
	if err != nil {
		return false, err
	}
	if fileInfo.Mode()&os.ModeSocket != 0 {
		return true, nil
	}
	return false, nil
}
