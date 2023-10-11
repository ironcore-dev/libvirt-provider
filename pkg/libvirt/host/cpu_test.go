// Copyright 2023 OnMetal authors
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

package host

import (
	. "github.com/onsi/gomega"

	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Testing Host CPU", func() {
	Context("CpuTopology", func() {
		It("Should return expected CPU topology", func() {
			receivedTopology, err := CpuTopology(lv)
			Expect(err).ToNot(HaveOccurred())
			Expect(receivedTopology).To(Equal(map[int][]int{
				0: {0, 1, 2, 3},
				1: {4, 5, 6, 7}}))
		})
	})

	Context("VMCPUPins", func() {
		It("Should return expected VM CPU pins", func() {
			receivedPins, err := VMCPUPins(lv)
			Expect(err).ToNot(HaveOccurred())
			Expect(receivedPins).To(Equal(map[int]int{
				0: 1,
				1: 1}))
		})
	})
})
