// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package sources

import (
	"context"
	"errors"
	"fmt"

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

func (c *CPU) Init(ctx context.Context) (sets.Set[core.ResourceName], error) {
	// To handle the limitations of floating-point arithmetic, where small rounding errors can occur
	// due to the finite precision of floating-point numbers.
	if c.overcommitVCPU < 1e-9 {
		return nil, errors.New("overcommitVCPU cannot be zero or negative")
	}

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

func (c *CPU) Allocate(requiredResources core.ResourceList) core.ResourceList {
	cpu, ok := requiredResources[core.ResourceCPU]
	if !ok {
		return nil
	}

	c.availableCPU.Sub(cpu)
	return core.ResourceList{core.ResourceCPU: cpu}
}

func (c *CPU) Deallocate(requiredResources core.ResourceList) []core.ResourceName {
	cpu, ok := requiredResources[core.ResourceCPU]
	if !ok {
		return nil
	}

	c.availableCPU.Add(cpu)
	return []core.ResourceName{core.ResourceCPU}
}

func (c *CPU) GetAvailableResource() core.ResourceList {
	return core.ResourceList{core.ResourceCPU: *c.availableCPU}.DeepCopy()
}
