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

type CPU struct {
	OvercommitVCPU float64
}

func NewSourceCPU(overCommitVCPU float64) *CPU {
	return &CPU{OvercommitVCPU: overCommitVCPU}
}

func (c *CPU) GetTotalResources(ctx context.Context) (core.ResourceList, error) {

	hostCPU, err := cpu.InfoWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get host cpu information: %w", err)
	}

	var hostCPUSum int64
	for _, v := range hostCPU {
		hostCPUSum += int64(v.Cores)
	}

	resources := core.ResourceList{core.ResourceCPU: *resource.NewScaledQuantity(hostCPUSum, resource.Kilo)}

	return resources, nil
}

func (c *CPU) GetName() core.ResourceName {
	return core.ResourceCPU
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

func (c *CPU) TuneTotalResources(resources core.ResourceList) error {
	if c.OvercommitVCPU == 0 || c.OvercommitVCPU < 0 {
		return errors.New("overcommitVCPU cannot be zero or negative")
	}

	cpuQuantity, ok := resources[core.ResourceCPU]
	if !ok {
		return errors.New("failed to get CPU resource from resource manager")
	}

	totalCPU, ok := cpuQuantity.AsInt64()
	if !ok {
		return errors.New("failed to convert CPU quantity to int64")
	}

	totalCPUFloat := float64(totalCPU) * c.OvercommitVCPU

	resources[core.ResourceCPU] = *resource.NewQuantity(int64(totalCPUFloat), resource.DecimalSI)

	return nil
}
