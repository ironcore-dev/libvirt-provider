// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package host_test

import (
	"context"

	"github.com/go-logr/logr"
	. "github.com/ironcore-dev/libvirt-provider/pkg/host"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"libvirt.org/go/libvirtxml"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var _ = Describe("Testing Numa", func() {
	Context("Align Memory", func() {
		It("Should return expected page size and page count when the memory is aligned", func() {
			receivedPageSize, receivedPages := AlignMemory(1000, 100)
			Expect(receivedPageSize).To(Equal(uint64(1000)))
			Expect(receivedPages).To(Equal(uint64(10)))
		})
		It("Should return expected page size and page count when the memory is not aligned", func() {
			receivedPageSize, receivedPages := AlignMemory(1001, 100)
			Expect(receivedPageSize).To(Equal(uint64(1100)))
			Expect(receivedPages).To(Equal(uint64(11)))
		})
	})

	Context("Numa tuner", func() {
		domain := &libvirtxml.Domain{
			Name: "NumaDomain",
			Memory: &libvirtxml.DomainMemory{
				Value: 10 * 1024 * 1024 * 1024, // 10Gi
				Unit:  Byte,
			},
			VCPU: &libvirtxml.DomainVCPU{Value: 2},
			MemoryBacking: &libvirtxml.DomainMemoryBacking{
				MemoryHugePages: &libvirtxml.DomainMemoryHugepages{},
			},
		}
		It("Should tune the numa tuner", func() {
			ctx := logr.NewContext(context.Background(), zap.New())
			n := &NumaTuner{
				Lv:           lv,
				HugePageSize: hugePageSize,
				CPUTopology: map[int][]int{
					0: {0, 1, 2, 3},
					1: {4, 5, 6, 7},
				},
			}
			Expect(n.Tune(ctx, domain, nil, nil)).To(Succeed())
		})
		It("Should return expected domain cpu tune", func() {
			Expect(domain.CPUTune).To(Equal(&libvirtxml.DomainCPUTune{
				VCPUPin: []libvirtxml.DomainCPUTuneVCPUPin{
					{VCPU: 0, CPUSet: "4"},
					{VCPU: 1, CPUSet: "5"},
				},
			}))
		})
		It("Should return expected numa tune", func() {
			Expect(domain.NUMATune).To(Equal(&libvirtxml.DomainNUMATune{
				Memory: &libvirtxml.DomainNUMATuneMemory{
					Mode:    "strict",
					Nodeset: "1",
				},
				MemNodes: []libvirtxml.DomainNUMATuneMemNode{
					{
						CellID:  0,
						Mode:    "strict",
						Nodeset: "1"},
				},
			}))
		})
		It("Should return expected cpu numa", func() {
			Expect(domain.CPU.Numa).To(Equal(&libvirtxml.DomainNuma{
				Cell: []libvirtxml.DomainCell{
					{
						ID:     &zero,
						CPUs:   "0,1",
						Memory: 10 * 1024 * 1024,
						Unit:   KiB},
				},
			}))
		})
	})

	Context("Clean blocked CPU", func() {
		It("Should return expected topology when there are no cpus to block", func() {
			receivedTopology := CleanBlockedCPUs(map[int][]int{
				0: {0, 1, 2, 3},
				1: {4, 5, 6, 7}},
				[]int{})
			Expect(receivedTopology).To(Equal(map[int][]int{
				0: {0, 1, 2, 3},
				1: {4, 5, 6, 7}}))
		})
		It("Should return expected topology when there are cpus to block", func() {
			receivedTopology := CleanBlockedCPUs(map[int][]int{
				0: {0, 1, 2, 3},
				1: {4, 5, 6, 7}},
				[]int{0, 2, 6})
			Expect(receivedTopology).To(Equal(map[int][]int{
				0: {1, 3},
				1: {4, 5, 7}}))
		})
	})
})
