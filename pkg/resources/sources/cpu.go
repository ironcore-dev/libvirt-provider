// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package sources

import (
	"context"
	"fmt"
	"math"

	"github.com/shirou/gopsutil/v3/cpu"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/sets"

	core "github.com/ironcore-dev/ironcore/api/core/v1alpha1"
)

const SourceCPU string = "cpu"

type CPU struct {
	overcommitVCPU float64
	availableCPU   *resource.Quantity
}

func NewSourceCPU(options Options) *CPU {
	return &CPU{overcommitVCPU: options.OvercommitVCPU}
}

func (c *CPU) GetName() string {
	return SourceCPU
}

// Modify rounding up cpu to total cores
func (c *CPU) Modify(resources core.ResourceList) error {
	cpu, ok := resources[core.ResourceCPU]
	if !ok {
		return fmt.Errorf("cannot found cpu in resources")
	}

	cpu.RoundUp(resource.Kilo)
	resources[core.ResourceCPU] = cpu

	return nil
}

func (c *CPU) CalculateMachineClassQuantity(requiredResources core.ResourceList) int64 {
	cpu, ok := requiredResources[core.ResourceCPU]
	if !ok {
		// this code cannot be call ever
		return 0
	}

	return int64(math.Ceil(float64(c.availableCPU.Value()) / float64(cpu.Value())))
}

func (c *CPU) Init(ctx context.Context) (sets.Set[core.ResourceName], error) {
	hostCPU, err := cpu.InfoWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get host cpu information: %w", err)
	}

	var hostCPUSum int64
	for _, v := range hostCPU {
		hostCPUSum += int64(v.Cores)
	}

	// Convert the calculated CPU quantity to an int64 to ensure that it represents a whole number of CPUs.
	cpuQuantity := int64(float64(hostCPUSum) * c.overcommitVCPU)
	c.availableCPU = resource.NewScaledQuantity(cpuQuantity, resource.Kilo)

	return sets.New(core.ResourceCPU), nil
}

func (c *CPU) Allocate(requiredResources core.ResourceList) (core.ResourceList, error) {
	cpu, ok := requiredResources[core.ResourceCPU]
	if !ok {
		return nil, nil
	}

	newCPU := *c.availableCPU
	newCPU.Sub(cpu)
	if newCPU.Sign() == -1 {
		return nil, fmt.Errorf("failed to allocate %s: %w", core.ResourceCPU, ErrResourceNotAvailable)
	}

	c.availableCPU = &newCPU
	return core.ResourceList{core.ResourceCPU: cpu}, nil
}

func (c *CPU) Deallocate(requiredResources core.ResourceList) []core.ResourceName {
	cpu, ok := requiredResources[core.ResourceCPU]
	if !ok {
		return nil
	}

	c.availableCPU.Add(cpu)
	return []core.ResourceName{core.ResourceCPU}
}

func (c *CPU) GetAvailableResources() core.ResourceList {
	return core.ResourceList{core.ResourceCPU: *c.availableCPU}
}
