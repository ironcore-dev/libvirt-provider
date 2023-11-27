// Copyright 2023 OnMetal authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mcr

import (
	"context"
	"fmt"
	"io"
	"math"
	"os"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
	"k8s.io/apimachinery/pkg/api/resource"

	ori "github.com/ironcore-dev/ironcore/iri/apis/machine/v1alpha1"
	"k8s.io/apimachinery/pkg/util/yaml"
)

func LoadMachineClasses(reader io.Reader) ([]ori.MachineClass, error) {
	var classList []ori.MachineClass
	if err := yaml.NewYAMLOrJSONDecoder(reader, 4096).Decode(&classList); err != nil {
		return nil, fmt.Errorf("unable to unmarshal machine classes: %w", err)
	}

	return classList, nil
}

func LoadMachineClassesFile(filename string) ([]ori.MachineClass, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("unable to open machine class file (%s): %w", filename, err)
	}

	return LoadMachineClasses(file)
}

func NewMachineClassRegistry(classes []ori.MachineClass) (*Mcr, error) {
	registry := Mcr{
		classes: map[string]ori.MachineClass{},
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
	classes map[string]ori.MachineClass
}

func (m *Mcr) Get(machineClassName string) (*ori.MachineClass, bool) {
	class, found := m.classes[machineClassName]
	return &class, found
}

func (m *Mcr) List() []*ori.MachineClass {
	var classes []*ori.MachineClass
	for name := range m.classes {
		class := m.classes[name]
		classes = append(classes, &class)
	}
	return classes
}

func GetQuantity(class *ori.MachineClass, host *Host) int64 {
	cpuRatio := host.Cpu.Value() / class.Capabilities.CpuMillis
	memoryRatio := host.Mem.Value() / class.Capabilities.MemoryBytes

	return int64(math.Min(float64(cpuRatio), float64(memoryRatio)))
}

func GetResources(ctx context.Context) (*Host, error) {
	hostMem, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get host memory: %w", err)
	}

	hostCPU, err := cpu.InfoWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get host memory: %w", err)
	}

	var hostCPUSum int64
	for _, v := range hostCPU {
		hostCPUSum += int64(v.Cores)
	}

	return &Host{
		Cpu: resource.NewScaledQuantity(hostCPUSum, resource.Kilo),
		Mem: resource.NewQuantity(int64(hostMem.Total), resource.BinarySI),
	}, nil
}

type Host struct {
	Cpu *resource.Quantity
	Mem *resource.Quantity
}
