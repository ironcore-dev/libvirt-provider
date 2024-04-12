// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package manager

/* require implement new logic for loading machineclasses

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ironcore-dev/libvirt-provider/pkg/api"
)

func returnEmptyMachineList(_ context.Context) ([]*api.Machine, error) {
	return nil, nil
}

var _ = Describe("Resource Manager", Ordered, func() {
	Context("without initialized manager", func() {
		machine := api.Machine{}
		It("should be failed with error ErrManagerNotInitialized", func() {
			resources := core.ResourceList{core.ResourceCPU: *resource.NewQuantity(1000, resource.DecimalSI)}
			Expect(Allocate(&machine, resources)).To(MatchError(ErrManagerNotInitialized))

			machine.Spec.Resources = resources
			Expect(Deallocate(&machine, machine.Spec.Resources.DeepCopy())).To(MatchError(ErrManagerNotInitialized))
		})

		It("should be return empty machine classes status", func() {
			Expect(GetMachineClassStatus()).Should(BeEmpty())
		})
	})

	Context("try initialized manager into incorrect state", func() {
		BeforeAll(func() {
			mng.reset()
		})

		It("should be fail", func() {
			By("initialize without list function")
			Expect(Initialize(context.TODO(), nil)).To(MatchError(ErrManagerListFuncInvalid))
			mng.reset()

			By("initialize without sources")
			Expect(Initialize(context.TODO(), returnEmptyMachineList)).To(MatchError(ErrManagerSourcesMissing))
			mng.reset()

			By("initialize with sources which manage same resources")
			Expect(AddSource(sources.NewSourceHugepages(sources.Options{}))).NotTo(HaveOccurred())
			Expect(AddSource(sources.NewSourceMemory(sources.Options{}))).NotTo(HaveOccurred())
			Expect(Initialize(context.TODO(), returnEmptyMachineList)).To(MatchError(ErrCommonResources))
			mng.reset()
		})
	})

	Context("with initialized manager", func() {
		totalResources := core.ResourceList{
			core.ResourceCPU:    *resource.NewQuantity(8000, resource.DecimalSI),
			core.ResourceMemory: *resource.NewQuantity(19327352832, resource.BinarySI),
		}

		BeforeAll(func() { mng.reset() })

		It("should initialize", func() {
			Expect(SetLogger(logger)).Should(Succeed())
			Expect(SetMachineClassesFilename("")).Should(Succeed())
			Expect(AddSource(sources.NewSourceDummy(totalResources, sources.Options{}))).Should(Succeed())
			Expect(Initialize(context.TODO(), returnEmptyMachineList)).Should(Succeed())
		})

		It("shouldn't be possible reinitialized or set parameters again", func() {
			Expect(SetLogger(logger)).ShouldNot(Succeed())
			Expect(SetMachineClassesFilename("")).ShouldNot(Succeed())
			Expect(AddSource(sources.NewSourceCPU(sources.Options{OvercommitVCPU: 1.0}))).ShouldNot(Succeed())
			Expect(Initialize(context.TODO(), returnEmptyMachineList)).ShouldNot(Succeed())
		})
	})
})

*/
