// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package sources

import (
	"context"

	core "github.com/ironcore-dev/ironcore/api/core/v1alpha1"
	"k8s.io/apimachinery/pkg/util/sets"
)

const (
	ResourceDummy core.ResourceName = "dummy"
	SourceDummy   string            = "dummy"
)

// Dummy source serves for dynamic change of available memory for unit tests
type Dummy struct {
	totalResources core.ResourceList
}

func NewSourceDummy(totalResources core.ResourceList, _ Options) *Dummy {
	return &Dummy{totalResources: totalResources}
}

func (d *Dummy) GetTotalResources(ctx context.Context) (core.ResourceList, error) {
	return d.totalResources, nil
}

func (d *Dummy) GetName() string {
	return SourceDummy
}

// Modify is dummy function
func (d *Dummy) Modify(_ core.ResourceList) error {
	return nil
}

func (d *Dummy) Allocate(requiredResources core.ResourceList) (core.ResourceList, error) {
	return nil, nil
}

func (d *Dummy) Deallocate(requiredResources core.ResourceList) []core.ResourceName {
	return nil
}

func (d *Dummy) GetAvailableResources() core.ResourceList {
	return core.ResourceList{core.ResourceCPU: *d.totalResources.CPU(), core.ResourceMemory: *d.totalResources.Memory()}.DeepCopy()
}

func (d *Dummy) Init(ctx context.Context) (sets.Set[core.ResourceName], error) {
	return nil, nil
}

func (d *Dummy) CalculateMachineClassQuantity(requiredResources core.ResourceList) int64 {
	return 0
}
