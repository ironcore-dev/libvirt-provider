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
	SourceMemory      string            = "memory"
	SourceHugepages   string            = "hugepages"
)

type Memory struct {
	availableMemory    *resource.Quantity
	reservedMemorySize MemorySize
}

func NewSourceMemory(options Options) *Memory {
	return &Memory{reservedMemorySize: options.ReservedMemorySize}
}

func (m *Memory) GetName() string {
	return SourceMemory
}

// Modify is dummy function
func (m *Memory) Modify(_ core.ResourceList) error {
	return nil
}

func (m *Memory) Init(ctx context.Context) (sets.Set[core.ResourceName], error) {
	hostMem, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get host memory information: %w", err)
	}

	availableMemory, err := calculateAvailableMemory(MemorySize(hostMem.Total), m.reservedMemorySize)
	if err != nil {
		return nil, err
	}
	m.availableMemory = availableMemory

	return sets.New(core.ResourceMemory), nil
}

func (m *Memory) Allocate(requiredResources core.ResourceList) core.ResourceList {
	mem, ok := requiredResources[core.ResourceMemory]
	if !ok {
		return nil
	}

	m.availableMemory.Sub(mem)
	return core.ResourceList{core.ResourceMemory: mem}
}

func (m *Memory) Deallocate(requiredResources core.ResourceList) []core.ResourceName {
	mem, ok := requiredResources[core.ResourceMemory]
	if !ok {
		return nil
	}

	m.availableMemory.Add(mem)
	return []core.ResourceName{core.ResourceMemory}
}

func (m *Memory) GetAvailableResource() core.ResourceList {
	return core.ResourceList{core.ResourceMemory: *m.availableMemory}.DeepCopy()
}

type Hugepages struct {
	pageSize           uint64
	pageCount          uint64
	availableMemory    *resource.Quantity
	availableHugePages *resource.Quantity
	blockedCount       uint64
	reservedMemorySize MemorySize
}

func NewSourceHugepages(options Options) *Hugepages {
	return &Hugepages{blockedCount: options.BlockedHugepages, reservedMemorySize: options.ReservedMemorySize}
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

	availableMemory, err := calculateAvailableMemory(MemorySize(m.pageSize*m.pageCount), m.reservedMemorySize)
	if err != nil {
		return nil, err
	}
	m.availableMemory = availableMemory

	availableHugepages, err := calculateAvailableHugepages(m.pageCount, m.blockedCount)
	if err != nil {
		return nil, err
	}
	m.availableHugePages = availableHugepages

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

func calculateAvailableMemory(totalMemory, reservedMemory MemorySize) (*resource.Quantity, error) {
	if reservedMemory >= totalMemory {
		return nil, fmt.Errorf("reservedMemorySize cannot be greater than equal to totalMemory: %v", resource.NewQuantity(int64(totalMemory), resource.BinarySI))
	}
	availableMemoryUint := MemorySize(totalMemory) - reservedMemory

	return resource.NewQuantity(int64(availableMemoryUint), resource.BinarySI), nil
}

func calculateAvailableHugepages(totalHugepages, blockedHugepages uint64) (*resource.Quantity, error) {
	if blockedHugepages >= totalHugepages {
		return nil, fmt.Errorf("blockedHugepages cannot be greater than equal to totalPage count: %d", totalHugepages)
	}
	availableHugepagesUint := totalHugepages - blockedHugepages

	return resource.NewQuantity(int64(availableHugepagesUint), resource.DecimalSI), nil
}