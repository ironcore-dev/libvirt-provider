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
)

const (
	ResourceHugepages core.ResourceName = "hugepages"
	SourceMemory      string            = "memory"
	SourceHugepages   string            = "hugepages"
)

type Memory struct{}

func NewSourceMemory(_ Options) *Memory {
	return &Memory{}
}

func (m *Memory) GetTotalResources(ctx context.Context) (core.ResourceList, error) {
	hostMem, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get host memory information: %w", err)
	}

	resources := core.ResourceList{core.ResourceMemory: *resource.NewQuantity(int64(hostMem.Total), resource.BinarySI)}

	return resources, nil
}

func (m *Memory) GetName() string {
	return SourceMemory
}

// Modify is dummy function
func (m *Memory) Modify(_ core.ResourceList) error {
	return nil
}

type Hugepages struct {
	pageSize  uint64
	pageCount uint64
}

func NewSourceHugepages(_ Options) *Hugepages {
	return &Hugepages{}
}

func (m *Hugepages) GetTotalResources(ctx context.Context) (core.ResourceList, error) {
	hostMem, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get host memory information: %w", err)
	}

	m.pageSize = hostMem.HugePageSize
	m.pageCount = hostMem.HugePagesTotal

	resources := core.ResourceList{
		core.ResourceMemory: *resource.NewQuantity(int64(m.pageSize*m.pageCount), resource.BinarySI),
		ResourceHugepages:   *resource.NewQuantity(int64(m.pageCount), resource.DecimalSI),
	}

	return resources, nil
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
