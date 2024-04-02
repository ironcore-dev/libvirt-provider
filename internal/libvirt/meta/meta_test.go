// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package meta_test

import (
	"encoding/xml"

	. "github.com/ironcore-dev/libvirt-provider/internal/libvirt/meta"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const libvirtProviderURL = "https://github.com/ironcore-dev/libvirt-provider"

var _ = Describe("LibvirtProviderMetadata", func() {
	Context("Marshalling", func() {
		It("marshals LibvirtProviderMetadata correctly when metadata is populated", func() {
			metadata := &LibvirtProviderMetadata{
				IRIMmachineLabels: "test-labels",
			}

			data, err := xml.Marshal(metadata)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(data)).To(Equal(createExpectedXML("test-labels")))
		})

		It("marshals LibvirtProviderMetadata correctly when no metadata present", func() {
			metadata := &LibvirtProviderMetadata{}

			data, err := xml.Marshal(metadata)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(data)).To(Equal(createExpectedXML("")))
		})
	})

	Context("Unmarshalling", func() {
		It("unmarshals XML to LibvirtProviderMetadata correctly when metadata is populated", func() {
			metadata := &LibvirtProviderMetadata{}
			Expect(xml.Unmarshal([]byte(createExpectedXML("test-labels")), metadata)).To(Succeed())
			Expect(metadata).To(Equal(&LibvirtProviderMetadata{
				IRIMmachineLabels: "test-labels",
			}))
		})

		It("unmarshals XML to LibvirtProviderMetadata correctly when no metadata present", func() {
			metadata := &LibvirtProviderMetadata{}
			Expect(xml.Unmarshal([]byte(createExpectedXML("")), metadata)).To(Succeed())
			Expect(metadata).To(Equal(&LibvirtProviderMetadata{}))
		})
	})

	Context("IRIMachineLabelsEncoder", func() {
		It("encodes IRIMachineLabels correctly when labels are populated", func() {
			labels := map[string]string{
				"machinepoollet.ironcore.dev/machine-uid":                         "test-uid",
				"downward-api.machinepoollet.ironcore.dev/root-machine-name":      "root-test-name",
				"downward-api.machinepoollet.ironcore.dev/root-machine-namespace": "root-test-namespace",
				"downward-api.machinepoollet.ironcore.dev/root-machine-uid":       "root-test-uid",
				"machinepoollet.ironcore.dev/machine-namespace":                   "test-namespace",
				"machinepoollet.ironcore.dev/machine-name":                        "test-name",
			}

			data := IRIMachineLabelsEncoder(labels)

			Expect(data).To(ContainSubstring(`"machinepoollet.ironcore.dev/machine-uid": "test-uid"`))
			Expect(data).To(ContainSubstring(`"downward-api.machinepoollet.ironcore.dev/root-machine-name": "root-test-name"`))
			Expect(data).To(ContainSubstring(`"downward-api.machinepoollet.ironcore.dev/root-machine-namespace": "root-test-namespace"`))
			Expect(data).To(ContainSubstring(`"downward-api.machinepoollet.ironcore.dev/root-machine-uid": "root-test-uid"`))
			Expect(data).To(ContainSubstring(`"machinepoollet.ironcore.dev/machine-namespace": "test-namespace"`))
			Expect(data).To(ContainSubstring(`"machinepoollet.ironcore.dev/machine-name": "test-name"`))
		})
	})
})

func createExpectedXML(labels string) string {
	return `<libvirtprovider:metadata xmlns:libvirtprovider="` + libvirtProviderURL + `"><libvirtprovider:irimachinelabels>` + labels + `</libvirtprovider:irimachinelabels></libvirtprovider:metadata>`
}
