// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package device

import "github.com/ironcore-dev/libvirt-provider/api"

type Plugin interface {
	Claim() (*api.PCIAddress, error)
	Release(pciAddress api.PCIAddress) error
	Init() error
}
