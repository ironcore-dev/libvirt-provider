// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package device

type Plugin interface {
	Claim() (string, error)
	Release(pci string) error
	Init() error
}
