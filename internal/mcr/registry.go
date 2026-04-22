// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package mcr

import (
	"context"
	"fmt"
	"io"
	"math"
	"os"

	computev1alpha1 "github.com/ironcore-dev/ironcore/api/compute/v1alpha1"
	corev1alpha1 "github.com/ironcore-dev/ironcore/api/core/v1alpha1"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/yaml"
)

// TODO drop once cpu and memory is handled by resource manager
const (
	ResourceCPU    = string(corev1alpha1.ResourceCPU)
	ResourceMemory = string(corev1alpha1.ResourceMemory)
)

type MachineClass struct {
	Name      string
	Resources map[string]int64
}

func LoadMachineClasses(reader io.Reader) ([]*MachineClass, error) {
	var machineClasses []computev1alpha1.MachineClass
	if err := yaml.NewYAMLOrJSONDecoder(reader, 4096).Decode(&machineClasses); err != nil {
		return nil, fmt.Errorf("unable to unmarshal machine classes: %w", err)
	}

	classes := make([]*MachineClass, 0, len(machineClasses))
	for i, mc := range machineClasses {
		if mc.Name == "" {
			return nil, fmt.Errorf("machine class at index %d has empty name", i)
		}
		resources := make(map[string]int64, len(mc.Capabilities))
		for k, v := range mc.Capabilities {
			if v.Value() <= 0 {
				return nil, fmt.Errorf("machine class %q has non-positive value for capability %q", mc.Name, k)
			}
			resources[string(k)] = v.Value()
		}
		classes = append(classes, &MachineClass{
			Name:      mc.Name,
			Resources: resources,
		})
	}
	return classes, nil
}

func LoadMachineClassesFile(filename string) ([]*MachineClass, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("unable to open machine class file (%s): %w", filename, err)
	}
	defer file.Close()

	return LoadMachineClasses(file)
}

func NewMachineClassRegistry(classes []*MachineClass) (*Mcr, error) {
	registry := Mcr{
		classes: map[string]*MachineClass{},
	}

	for _, class := range classes {
		if _, ok := registry.classes[class.Name]; ok {
			return nil, fmt.Errorf("multiple classes with same name (%s) found", class.Name)
		}
		registry.classes[class.Name] = class
	}

	return &registry, nil
}

type Mcr struct {
	classes map[string]*MachineClass
}

func (m *Mcr) Get(machineClassName string) (*MachineClass, bool) {
	class, found := m.classes[machineClassName]
	return class, found
}

func (m *Mcr) List() []*MachineClass {
	var classes []*MachineClass
	for name := range m.classes {
		classes = append(classes, m.classes[name])
	}
	return classes
}

func GetQuantity(class *MachineClass, host *Host) int64 {
	cpuRatio := host.Cpu.Value() / class.Resources[ResourceCPU]
	memoryRatio := host.Mem.Value() / class.Resources[ResourceMemory]

	return int64(math.Min(float64(cpuRatio), float64(memoryRatio)))
}

func GetResources(ctx context.Context, enableHugepages bool) (*Host, error) {
	hostMem, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get host memory information: %w", err)
	}

	hostCPU, err := cpu.InfoWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get host cpu information: %w", err)
	}

	var hostCPUSum int64
	for _, v := range hostCPU {
		hostCPUSum += int64(v.Cores)
	}

	host := &Host{
		Cpu: resource.NewQuantity(hostCPUSum, resource.DecimalSI),
	}

	if enableHugepages {
		host.Mem = resource.NewQuantity(int64(hostMem.HugePagesTotal*hostMem.HugePageSize), resource.BinarySI)
	} else {
		host.Mem = resource.NewQuantity(int64(hostMem.Total), resource.BinarySI)
	}

	return host, nil
}

type Host struct {
	Cpu *resource.Quantity
	Mem *resource.Quantity
}
