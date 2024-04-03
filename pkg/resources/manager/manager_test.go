// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/ironcore-dev/libvirt-provider/pkg/api"
	"github.com/ironcore-dev/libvirt-provider/pkg/resources/sources"

	core "github.com/ironcore-dev/ironcore/api/core/v1alpha1"
	iriv1alpha1 "github.com/ironcore-dev/ironcore/iri/apis/machine/v1alpha1"
)

func returnEmptyMachineList(_ context.Context) ([]*api.Machine, error) {
	return nil, nil
}

var _ = Describe("Resource Manager", Ordered, func() {
	machineClasses := []*iriv1alpha1.MachineClass{
		{
			Name: "machineClassx3xlarge",
			Capabilities: &iriv1alpha1.MachineClassCapabilities{
				CpuMillis:   4000,
				MemoryBytes: 8589934592,
			},
		},
		{
			Name: "machineClassx2medium",
			Capabilities: &iriv1alpha1.MachineClassCapabilities{
				CpuMillis:   2000,
				MemoryBytes: 2147483648,
			},
		},
	}
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
			Expect(Initialize(context.TODO(), returnEmptyMachineList)).To(MatchError(ErrResourceAlreadyRegistered))
			mng.reset()

			By("initialize without resources")
			Expect(AddSource(sources.NewSourceDummy(nil, sources.Options{}))).NotTo(HaveOccurred())
			Expect(SetMachineClasses(machineClasses)).NotTo(HaveOccurred())
			Expect(Initialize(context.TODO(), returnEmptyMachineList)).Should(MatchError(ErrResourceUnsupported))
			mng.reset()
		})
	})

	Context("with initialized manager", func() {
		totalResources := core.ResourceList{
			core.ResourceCPU:    *resource.NewQuantity(8000, resource.DecimalSI),
			core.ResourceMemory: *resource.NewQuantity(19327352832, resource.BinarySI),
		}
		machine := api.Machine{}
		BeforeAll(func() { mng.reset() })

		It("should initialize", func() {
			Expect(SetLogger(logger)).Should(Succeed())
			Expect(SetMachineClasses(machineClasses)).Should(Succeed())
			Expect(AddSource(sources.NewSourceDummy(totalResources, sources.Options{}))).Should(Succeed())
			Expect(Initialize(context.TODO(), returnEmptyMachineList)).Should(Succeed())
		})

		It("shouldn't be possible reinitialized or set parameters again", func() {
			Expect(SetLogger(logger)).ShouldNot(Succeed())
			Expect(SetMachineClasses(nil)).ShouldNot(Succeed())
			Expect(AddSource(sources.NewSourceCPU(sources.Options{OvercommitVCPU: 1.0}))).ShouldNot(Succeed())
			Expect(Initialize(context.TODO(), returnEmptyMachineList)).ShouldNot(Succeed())
		})

		It("should allocate one machine", func() {
			requiredResource, err := GetMachineClassRequiredResources(machineClasses[0].GetName())
			Expect(err).ShouldNot(HaveOccurred())
			Expect(requiredResource).NotTo(BeEmpty())
			Expect(Allocate(&machine, requiredResource)).NotTo(HaveOccurred())
			Expect(machine.Spec.Resources).Should(HaveLen(2))
			Expect(mng.resourcesAvailable).Should(Equal(core.ResourceList{
				core.ResourceCPU:    *resource.NewQuantity(4000, resource.DecimalSI),
				core.ResourceMemory: *resource.NewQuantity(10737418240, resource.BinarySI),
			}))
		})
	})
})
