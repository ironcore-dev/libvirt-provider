// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package sources

import (
	"context"

	core "github.com/ironcore-dev/ironcore/api/core/v1alpha1"
)

// Dummy source serve for unit tests
type Dummy struct {
	totalResources core.ResourceList
}

func NewSourceDummy() *Dummy {
	return &Dummy{
		totalResources: core.ResourceList{},
	}
}

func (d *Dummy) GetTotalResources(ctx context.Context) (core.ResourceList, error) {
	return d.totalResources, nil
}

func (d *Dummy) GetName() string {
	return "dummy"
}

// Modify is dummy function
func (d *Dummy) Modify(_ core.ResourceList) error {
	return nil
}

func (d *Dummy) SetTotalResources(resources core.ResourceList) {
	for key, quantity := range resources {
		d.totalResources[key] = quantity
	}
}
