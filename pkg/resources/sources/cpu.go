// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package sources

import (
	"context"
	"errors"
	"fmt"

	"github.com/shirou/gopsutil/v3/cpu"
	"k8s.io/apimachinery/pkg/api/resource"

	core "github.com/ironcore-dev/ironcore/api/core/v1alpha1"
)

const SourceCPU string = "cpu"

type CPU struct {
	OvercommitVCPU float64
}

func NewSourceCPU(options Options) *CPU {
	return &CPU{OvercommitVCPU: options.OvercommitVCPU}
}

func (c *CPU) GetTotalResources(ctx context.Context) (core.ResourceList, error) {
	// To handle the limitations of floating-point arithmetic, where small rounding errors can occur
	// due to the finite precision of floating-point numbers.
	if c.OvercommitVCPU < 1e-9 {
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
	cpuQuantity := int64(float64(hostCPUSum) * c.OvercommitVCPU)
	resources := core.ResourceList{
		core.ResourceCPU: *resource.NewScaledQuantity(cpuQuantity, resource.Kilo),
	}

	return resources, nil
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
