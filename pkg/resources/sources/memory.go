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
	SourceMemory string = "memory"
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

func (m *Memory) CalculateMachineClassQuantity(requiredResources core.ResourceList) int64 {
	mem, ok := requiredResources[core.ResourceMemory]
	if !ok {
		// this code cannot be call ever
		return 0
	}

	return int64(math.Ceil(float64(m.availableMemory.Value()) / float64(mem.Value())))
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

func (m *Memory) Allocate(requiredResources core.ResourceList) (core.ResourceList, error) {
	mem, ok := requiredResources[core.ResourceMemory]
	if !ok {
		return nil, nil
	}

	newMem := *m.availableMemory
	newMem.Sub(mem)

	if newMem.Sign() == -1 {
		return nil, fmt.Errorf("failed to allocate resource %s: %w", core.ResourceMemory, ErrResourceNotAvailable)
	}

	m.availableMemory = &newMem
	return core.ResourceList{core.ResourceMemory: mem}, nil
}

func (m *Memory) Deallocate(requiredResources core.ResourceList) []core.ResourceName {
	mem, ok := requiredResources[core.ResourceMemory]
	if !ok {
		return nil
	}

	m.availableMemory.Add(mem)
	return []core.ResourceName{core.ResourceMemory}
}

func (m *Memory) GetAvailableResources() core.ResourceList {
	return core.ResourceList{core.ResourceMemory: *m.availableMemory}
}

func calculateAvailableMemory(totalMemory, reservedMemory MemorySize) (*resource.Quantity, error) {
	if reservedMemory > totalMemory {
		return nil, fmt.Errorf("reservedMemorySize cannot be greater than totalMemory: %v", resource.NewQuantity(int64(totalMemory), resource.BinarySI))
	}
	availableMemoryUint := MemorySize(totalMemory) - reservedMemory

	return resource.NewQuantity(int64(availableMemoryUint), resource.BinarySI), nil
}
