// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	core "github.com/ironcore-dev/ironcore/api/core/v1alpha1"
	iri "github.com/ironcore-dev/ironcore/iri/apis/machine/v1alpha1"
)

type MachineClass struct {
	Name         string            `json:"name"`
	Capabilities core.ResourceList `json:"capabilities"`
	available    int64
	iriClass     *iri.MachineClass
}
