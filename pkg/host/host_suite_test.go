// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package host_test

import (
	"testing"
	"time"

	"github.com/digitalocean/go-libvirt"
	"github.com/ironcore-dev/libvirt-provider/pkg/api"
	"github.com/ironcore-dev/libvirt-provider/pkg/host"
	mockdialer "github.com/ironcore-dev/libvirt-provider/pkg/mocks/libvirt_dialer"
	"github.com/ironcore-dev/libvirt-provider/pkg/store"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	tmpDir       string
	machineStore store.Store[*api.Machine]
	dialer       *mockdialer.MockLibvirtDilaer
	lv           *libvirt.Libvirt
	zero         uint = 0
)

const (
	eventuallyTimeout    = 5 * time.Second
	pollingInterval      = 250 * time.Millisecond
	consistentlyDuration = 1 * time.Second
	hugePageSize         = 1048576
)

func TestServer(t *testing.T) {
	SetDefaultConsistentlyPollingInterval(pollingInterval)
	SetDefaultEventuallyPollingInterval(pollingInterval)
	SetDefaultEventuallyTimeout(eventuallyTimeout)
	SetDefaultConsistentlyDuration(consistentlyDuration)

	RegisterFailHandler(Fail)
	RunSpecs(t, "Host Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	var err error

	tmpDir = GinkgoT().TempDir()

	machineStore, err = host.NewStore[*api.Machine](host.Options[*api.Machine]{
		Dir: tmpDir,
		NewFunc: func() *api.Machine {
			return &api.Machine{}
		},
	})
	Expect(err).NotTo(HaveOccurred())

	domainsList := []libvirt.Domain{
		{
			Name: "Domain1",
			UUID: mockdialer.NewUUID(),
			ID:   1,
		},
	}

	dialer = mockdialer.NewMockDialer(domainsList)
	lv = libvirt.NewWithDialer(dialer)
	Expect(lv.Connect()).To(Succeed())
})

var _ = AfterSuite(func() {
	Expect(lv.ConnectClose()).To(Succeed())
	Expect(dialer.Close()).To(Succeed())
})
