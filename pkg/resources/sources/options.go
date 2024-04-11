// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package sources

import (
	"fmt"
	"strconv"
)

type Options struct {
	Sources            []string
	OvercommitVCPU     float64
	ReservedMemorySize MemorySize
	BlockedHugepages   uint64
}

// MemorySize is a custom type to handle memory sizes in human-readable format.
type MemorySize uint64

func (m *MemorySize) String() string {
	return strconv.FormatUint(uint64(*m), 10)
}

var unitMap = map[string]uint64{
	"Ki": 1024,
	"Mi": 1024 * 1024,
	"Gi": 1024 * 1024 * 1024,
}

func (m *MemorySize) Set(s string) error {
	// Parse the input string to extract the size and unit.
	var size uint64
	var unit string
	_, err := fmt.Sscanf(s, "%d%s", &size, &unit)
	if err != nil {
		return err
	}
	// Convert the size to bytes based on the unit.
	factor, ok := unitMap[unit]
	if !ok {
		return fmt.Errorf("unsupported unit: %s. Supported units are: %v", unit, GetmemorySizeUnitsAvailable())
	}
	*m = MemorySize(size * factor)
	return nil
}

func GetmemorySizeUnitsAvailable() (units []string) {
	for k := range unitMap {
		units = append(units, k)
	}
	return units
}

func (m *MemorySize) Type() string {
	return ""
}
