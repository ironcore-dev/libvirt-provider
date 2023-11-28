// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package meta_test

import (
	"encoding/xml"

	. "github.com/ironcore-dev/libvirt-provider/pkg/meta"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/utils/ptr"
)

const expectedXML = `<virtlet:metadata xmlns:virtlet="https://github.com/onmetal/virtlet"><virtlet:namespace>foo</virtlet:namespace><virtlet:name>bar</virtlet:name></virtlet:metadata>`
const sgxExpectedXML = `<virtlet:metadata xmlns:virtlet="https://github.com/onmetal/virtlet"><virtlet:namespace>foo</virtlet:namespace><virtlet:name>bar</virtlet:name><virtlet:sgx_memory>68719476736</virtlet:sgx_memory><virtlet:sgx_node>0</virtlet:sgx_node></virtlet:metadata>`

var _ = Describe("Meta", func() {
	Context("VirtletMetadata", func() {
		Describe("Marshal", func() {
			It("should correctly marshal the metadata", func() {
				metadata := &VirtletMetadata{
					Namespace: "foo",
					Name:      "bar",
				}

				data, err := xml.Marshal(metadata)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(data)).To(Equal(expectedXML))
			})
		})

		Describe("Unmarshal", func() {
			It("should correctly unmarshal the metadata", func() {
				metadata := &VirtletMetadata{}
				Expect(xml.Unmarshal([]byte(expectedXML), metadata)).To(Succeed())
				Expect(metadata).To(Equal(&VirtletMetadata{
					Namespace: "foo",
					Name:      "bar",
				}))
			})
		})

		Describe("SGX Marshal", func() {
			It("should correctly marshal the SGX metadata", func() {
				metadata := &VirtletMetadata{
					Namespace: "foo",
					Name:      "bar",
					SGXMemory: ptr.To[int64](68719476736),
					SGXNode:   ptr.To[int](0),
				}

				data, err := xml.Marshal(metadata)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(data)).To(Equal(sgxExpectedXML))
			})
		})

		Describe("SGX Unmarshal", func() {
			It("should correctly unmarshal the SGX metadata", func() {
				metadata := &VirtletMetadata{}
				Expect(xml.Unmarshal([]byte(sgxExpectedXML), metadata)).To(Succeed())
				Expect(metadata).To(Equal(&VirtletMetadata{
					Namespace: "foo",
					Name:      "bar",
					SGXMemory: ptr.To[int64](68719476736),
					SGXNode:   ptr.To[int](0),
				}))
			})
		})
	})
})
