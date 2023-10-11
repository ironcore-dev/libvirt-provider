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
	"fmt"
	"io"
	"os"

	ori "github.com/onmetal/onmetal-api/ori/apis/machine/v1alpha1"
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
