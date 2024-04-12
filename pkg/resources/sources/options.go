// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package sources

import (
	"github.com/docker/go-units"
)

type Options struct {
	Sources            []string
	OvercommitVCPU     float64
	ReservedMemorySize MemorySize
	BlockedHugepages   uint64
}

// // MemorySize is a custom type to handle memory sizes in human-readable format.
type MemorySize uint64

func (m MemorySize) String() string {
	return units.BytesSize(float64(m))
}
func (m *MemorySize) Set(s string) error {
	size, err := units.RAMInBytes(s)
	if err != nil {
		return err
	}
	*m = MemorySize(size)
	return nil
}

func (m MemorySize) Type() string {
	return ""
}
