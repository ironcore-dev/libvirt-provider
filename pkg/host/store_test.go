// Copyright 2023 OnMetal authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package host_test

import (
	"os"
	"path/filepath"

	"github.com/ironcore-dev/libvirt-provider/pkg/api"
	"github.com/ironcore-dev/libvirt-provider/pkg/store"
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
