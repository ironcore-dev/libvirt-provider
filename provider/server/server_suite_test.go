// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package server_test

import (
	"testing"
	"time"

	iri "github.com/ironcore-dev/ironcore/iri/apis/machine/v1alpha1"
	"github.com/ironcore-dev/libvirt-provider/pkg/api"
	"github.com/ironcore-dev/libvirt-provider/pkg/host"
	"github.com/ironcore-dev/libvirt-provider/pkg/mcr"
	"github.com/ironcore-dev/libvirt-provider/pkg/utils"
	"github.com/ironcore-dev/libvirt-provider/provider/server"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	srv *server.Server
)

const (
	eventuallyTimeout    = 3 * time.Second
	pollingInterval      = 50 * time.Millisecond
	consistentlyDuration = 1 * time.Second
	baseURL              = "http://localhost:8080"
)

func TestServer(t *testing.T) {
	SetDefaultConsistentlyPollingInterval(pollingInterval)
	SetDefaultEventuallyPollingInterval(pollingInterval)
	SetDefaultEventuallyTimeout(eventuallyTimeout)
	SetDefaultConsistentlyDuration(consistentlyDuration)

	RegisterFailHandler(Fail)
	RunSpecs(t, "Server Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	tmpDir := GinkgoT().TempDir()
	machineStore, err := host.NewStore[*api.Machine](host.Options[*api.Machine]{
		Dir: tmpDir,
		NewFunc: func() *api.Machine {
			return &api.Machine{}
		},
		CreateStrategy: utils.MachineStrategy,
	})
	Expect(err).NotTo(HaveOccurred())

	machineClasses, err := mcr.NewMachineClassRegistry([]iri.MachineClass{
		{
			//TODO
			Name: "x3-xlarge",
			Capabilities: &iri.MachineClassCapabilities{
				CpuMillis:   4000,
				MemoryBytes: 8589934592,
			},
		},
	})
	Expect(err).NotTo(HaveOccurred())

	srv, err = server.New(server.Options{
		BaseURL:        baseURL,
		MachineStore:   machineStore,
		MachineClasses: machineClasses,
	})
	Expect(err).NotTo(HaveOccurred())
})
