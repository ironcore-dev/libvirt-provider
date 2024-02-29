// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package server_test

import (
	iriv1alpha1 "github.com/ironcore-dev/ironcore/iri/apis/machine/v1alpha1"
	"github.com/ironcore-dev/libvirt-provider/pkg/resources/manager"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Status", func() {
	It("should get list of supported machine class with calculated quantity in status", func(ctx SpecContext) {
		By("getting machine class status")
		statusResp, err := machineClient.Status(ctx, &iriv1alpha1.StatusRequest{})
		Expect(err).NotTo(HaveOccurred())
		/*
			By("loading machine classes from file")
			machineClasses, err := mcr.LoadMachineClasses(machineClassesFile)
			Expect(err).NotTo(HaveOccurred())
		*/
		By("getting host resources")
		classesStatus := manager.GetMachineClassStatus()
		Expect(err).NotTo(HaveOccurred())

		By("validating machine class and calculated quantity in MachineClassStatus")
		Expect(statusResp.MachineClassStatus).To(ContainElements(classesStatus[0], classesStatus[1]))
	})
})
