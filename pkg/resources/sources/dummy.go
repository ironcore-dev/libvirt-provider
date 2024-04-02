// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package sources

import (
	"context"

	core "github.com/ironcore-dev/ironcore/api/core/v1alpha1"
)

const ResourceDummy core.ResourceName = "dummy"

// Dummy source serves for dynamic change of available memory for unit tests
type Dummy struct {
	totalResources core.ResourceList
}

func NewSourceDummy(totalResources core.ResourceList) *Dummy {
	return &Dummy{totalResources: totalResources}
}

func (d *Dummy) GetTotalResources(ctx context.Context) (core.ResourceList, error) {
	return d.totalResources, nil
}

func (d *Dummy) GetName() core.ResourceName {
	return ResourceDummy
}

// Modify is dummy function
func (d *Dummy) Modify(_ core.ResourceList) error {
	return nil
}

func (d *Dummy) TuneTotalResources(_ core.ResourceList) error {
	return nil
}
