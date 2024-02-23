// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package api

import "k8s.io/apimachinery/pkg/api/resource"

type ResourceName string

const (
	ResourceCPU       ResourceName = "cpu"
	ResourceMemory    ResourceName = "memory"
	ResourceSGX       ResourceName = "sgx"
	ResourceHugepages ResourceName = "hugepages"
)

type ResourceList map[ResourceName]resource.Quantity

type NUMAPreferences struct {
	CPUNodes    []int `json:"cpuNodes,omitempty"`
	MemoryNodes []int `json:"memoryNodes,omitempty"`
	IoNodes     []int `json:"ioNodes,omitempty"`
}

type NUMAPlacement struct {
	CPUNodes    []int `json:"cpuNodes"`
	MemoryNodes []int `json:"memoryNodes"`
	IoNodes     []int `json:"ioNodes"`
}

// get returns the resource with name if specified, otherwise it returns a nil quantity with default format
func (rl *ResourceList) get(name ResourceName, defaultFormat resource.Format) *resource.Quantity {
	if val, ok := (*rl)[name]; ok {
		return &val
	}
	return &resource.Quantity{Format: defaultFormat}
}

// CPU is a shorthand for getting the quantity associated with ResourceCPU.
func (rl *ResourceList) CPU() *resource.Quantity {
	return rl.get(ResourceCPU, resource.DecimalSI)
}

// Memory is a shorthand for getting the quantity associated with ResourceMemory.
func (rl *ResourceList) Memory() *resource.Quantity {
	return rl.get(ResourceMemory, resource.BinarySI)
}

// SGX is a shorthand for getting the quantity associated with ResourceSGX.
func (rl *ResourceList) SGX() *resource.Quantity {
	return rl.get(ResourceSGX, resource.BinarySI)
}

// HugepagesEnabled is a shorthand for checking if hugepage is enabled.
func (rl *ResourceList) HugepagesEnabled() bool {
	return rl.get(ResourceHugepages, resource.DecimalSI).Value() == 1
}
