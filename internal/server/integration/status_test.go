// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package integration_test

import (
	corev1alpha1 "github.com/ironcore-dev/ironcore/api/core/v1alpha1"
	iriv1alpha1 "github.com/ironcore-dev/ironcore/iri/apis/machine/v1alpha1"
	"github.com/ironcore-dev/libvirt-provider/internal/mcr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Status", func() {
	It("should get list of supported machine class with calculated quantity in status", func(ctx SpecContext) {
		By("getting machine class status")
		statusResp, err := machineClient.Status(ctx, &iriv1alpha1.StatusRequest{})
		Expect(err).NotTo(HaveOccurred())

		By("loading machine classes from file")
		machineClasses, err := mcr.LoadMachineClasses(machineClassesFile)
		Expect(err).NotTo(HaveOccurred())

		By("getting host resources")
		hostResources, err := mcr.GetResources(ctx, false)
		Expect(err).NotTo(HaveOccurred())

		By("validating machine class and calculated quantity in MachineClassStatus")
		Expect(statusResp.MachineClassStatus).To(ContainElements(
			&iriv1alpha1.MachineClassStatus{
				MachineClass: &iriv1alpha1.MachineClass{
					Name: machineClasses[0].Name,
					Capabilities: &iriv1alpha1.MachineClassCapabilities{
						Resources: map[string]int64{
							string(corev1alpha1.ResourceCPU):    machineClasses[0].Capabilities.Resources[string(corev1alpha1.ResourceCPU)],
							string(corev1alpha1.ResourceMemory): machineClasses[0].Capabilities.Resources[string(corev1alpha1.ResourceMemory)],
						},
					},
				},
				Quantity: mcr.GetQuantity(machineClasses[0], hostResources),
			},
			&iriv1alpha1.MachineClassStatus{
				MachineClass: &iriv1alpha1.MachineClass{
					Name: machineClasses[1].Name,
					Capabilities: &iriv1alpha1.MachineClassCapabilities{
						Resources: map[string]int64{
							string(corev1alpha1.ResourceCPU):    machineClasses[1].Capabilities.Resources[string(corev1alpha1.ResourceCPU)],
							string(corev1alpha1.ResourceMemory): machineClasses[1].Capabilities.Resources[string(corev1alpha1.ResourceMemory)],
						},
					},
				},
				Quantity: mcr.GetQuantity(machineClasses[1], hostResources),
			},
		))
	})
})
