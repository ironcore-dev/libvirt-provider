// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package host_test

import (
	. "github.com/onsi/gomega"

	. "github.com/ironcore-dev/libvirt-provider/pkg/host"
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
