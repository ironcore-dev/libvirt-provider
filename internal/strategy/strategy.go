// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package strategy

import (
	"github.com/ironcore-dev/libvirt-provider/api"
)

var MachineStrategy = machineStrategy{}

type machineStrategy struct{}

func (machineStrategy) PrepareForCreate(obj *api.Machine) {
	obj.Status = api.MachineStatus{State: api.MachineStatePending}
}
