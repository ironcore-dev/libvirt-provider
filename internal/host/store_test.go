// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package host_test

import (
	"os"
	"path/filepath"

	"github.com/ironcore-dev/libvirt-provider/api"
	"github.com/ironcore-dev/libvirt-provider/internal/store"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Store", func() {

	It("should correctly create a object", func(ctx SpecContext) {
		By("creating a watch")
		watch, err := machineStore.Watch(ctx)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(watch.Stop)

		By("creating a machine object")
		machine, err := machineStore.Create(ctx, &api.Machine{
			Metadata: api.Metadata{
				ID: "test-id",
			},
			Spec:   api.MachineSpec{},
			Status: api.MachineStatus{},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(machine).NotTo(BeNil())

		By("checking that the store object exists")
		data, err := os.ReadFile(filepath.Join(tmpDir, machine.ID))
		Expect(err).NotTo(HaveOccurred())
		Expect(data).NotTo(BeNil())

		By("checking that the event got fired")
		event := &store.WatchEvent[*api.Machine]{
			Type:   store.WatchEventTypeCreated,
			Object: machine,
		}
		Eventually(watch.Events()).Should(Receive(event))
	})
})
