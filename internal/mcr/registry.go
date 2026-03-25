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
	"github.com/ironcore-dev/libvirt-provider/api"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/yaml"
)

const (
	ResourceCPU    = "cpu"
	ResourceMemory = "memory"
)

type MachineClass struct {
	Name      string
	Resources map[string]int64
}

func LoadMachineClasses(reader io.Reader) ([]*MachineClass, error) {
	var fileClasses []computev1alpha1.MachineClass
	if err := yaml.NewYAMLOrJSONDecoder(reader, 4096).Decode(&fileClasses); err != nil {
		return nil, fmt.Errorf("unable to unmarshal machine classes: %w", err)
	}

	classes := make([]*MachineClass, 0, len(fileClasses))
	for i, fc := range fileClasses {
		if fc.Name == "" {
			return nil, fmt.Errorf("machine class at index %d has empty name", i)
		}
		cpuQty := fc.Capabilities[corev1alpha1.ResourceCPU]
		memQty := fc.Capabilities[corev1alpha1.ResourceMemory]
		resources := map[string]int64{
			ResourceCPU:    cpuQty.Value(),
			ResourceMemory: memQty.Value(),
		}
		if gpu, ok := fc.Capabilities[api.NvidiaGPUPlugin]; ok {
			resources[api.NvidiaGPUPlugin] = gpu.Value()
		}
		classes = append(classes, &MachineClass{
			Name:      fc.Name,
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
