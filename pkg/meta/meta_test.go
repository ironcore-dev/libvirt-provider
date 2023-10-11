// Copyright 2022 OnMetal authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package meta_test

import (
	"encoding/xml"

	. "github.com/onmetal/virtlet/meta"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/utils/pointer"
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
					SGXMemory: pointer.Int64(68719476736),
					SGXNode:   pointer.Int(0),
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
					SGXMemory: pointer.Int64(68719476736),
					SGXNode:   pointer.Int(0),
				}))
			})
		})
	})
})
