// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package sources

import (
	"context"
	"fmt"
	"math"

	core "github.com/ironcore-dev/ironcore/api/core/v1alpha1"
	"github.com/shirou/gopsutil/v3/mem"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/sets"
)

const (
	ResourceHugepages core.ResourceName = "hugepages"
	SourceHugepages   string            = "hugepages"
)

type Hugepages struct {
	pageSize           uint64
	pageCount          uint64
	availableMemory    *resource.Quantity
	availableHugePages *resource.Quantity
	blockedCount       uint64
}

func NewSourceHugepages(options Options) *Hugepages {
	return &Hugepages{blockedCount: options.BlockedHugepages}
}

func (m *Hugepages) GetName() string {
	return SourceHugepages
}

// Modify set hugepages for resources and rounded up memory size
func (m *Hugepages) Modify(resources core.ResourceList) error {
	memory, ok := resources[core.ResourceMemory]
	if !ok {
		return fmt.Errorf("cannot found memory in resources")
	}

	if memory.Value() <= 0 {
		return fmt.Errorf("invalid value of memory resource %d", memory.Value())
	}

	size := float64(memory.Value())
	hugepages := uint64(math.Ceil(size / float64(m.pageSize)))
	resources[ResourceHugepages] = *resource.NewQuantity(int64(hugepages), resource.DecimalSI)
	// i don't want to do rounding
	resources[core.ResourceMemory] = *resource.NewQuantity(int64(hugepages)*int64(m.pageSize), resource.DecimalSI)

	return nil
}

func (m *Hugepages) Init(ctx context.Context) (sets.Set[core.ResourceName], error) {
	hostMem, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get host memory information: %w", err)
	}

	m.pageSize = hostMem.HugePageSize
	m.pageCount = hostMem.HugePagesTotal

	availableHugepagesCount, err := calculateAvailableHugepages(m.pageCount, m.blockedCount)
	if err != nil {
		return nil, err
	}
	m.availableHugePages = resource.NewQuantity(int64(availableHugepagesCount), resource.DecimalSI)
	m.availableMemory = resource.NewQuantity(int64(availableHugepagesCount*m.pageSize), resource.BinarySI)

	return sets.New(core.ResourceMemory, ResourceHugepages), nil
}

func (m *Hugepages) Allocate(requiredResources core.ResourceList) core.ResourceList {
	mem, ok := requiredResources[core.ResourceMemory]
	if !ok {
		return nil
	}

	m.availableMemory.Sub(mem)

	hugepages, ok := requiredResources[ResourceHugepages]
	if !ok {
		return core.ResourceList{core.ResourceMemory: mem}
	}

	m.availableHugePages.Sub(hugepages)

	return core.ResourceList{core.ResourceMemory: mem, ResourceHugepages: hugepages}
}

func (m *Hugepages) Deallocate(requiredResources core.ResourceList) []core.ResourceName {
	mem, ok := requiredResources[core.ResourceMemory]
	if !ok {
		return nil
	}

	m.availableMemory.Add(mem)

	hugepages, ok := requiredResources[ResourceHugepages]
	if !ok {
		return []core.ResourceName{core.ResourceMemory}
	}

	m.availableHugePages.Add(hugepages)

	return []core.ResourceName{core.ResourceMemory, ResourceHugepages}
}

func (m *Hugepages) GetAvailableResource() core.ResourceList {
	return core.ResourceList{core.ResourceMemory: *m.availableMemory, ResourceHugepages: *m.availableHugePages}.DeepCopy()
}

func calculateAvailableHugepages(totalHugepages, blockedHugepages uint64) (uint64, error) {
	if blockedHugepages > totalHugepages {
		return 0, fmt.Errorf("blockedHugepages cannot be greater than totalPage count: %d", totalHugepages)
	}

	return totalHugepages - blockedHugepages, nil
}
