// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"context"

	core "github.com/ironcore-dev/ironcore/api/core/v1alpha1"
	"github.com/ironcore-dev/libvirt-provider/pkg/api"
)

type Source interface {
	// GetName return name of source, ideally it has to be uniq
	GetName() string
	// Modify serves for modification resources base (rounding, create subresource).
	// Example: Machineclasses contains memory size only, but if libvirt provider will use hugepages source.
	//   Memory size has to be rounded to whole hugepages and it will create additional resource which count of hugepages.
	Modify(core.ResourceList) error
	Init(context.Context) error
	Allocate(core.ResourceList) core.ResourceList
	Deallocate(core.ResourceList) []core.ResourceName
	GetAvailableResource() core.ResourceList
}

type NumaScheduler interface {
	Pin(cores uint, machine *api.Machine) error
	Unpin(machine *api.Machine) error
}
