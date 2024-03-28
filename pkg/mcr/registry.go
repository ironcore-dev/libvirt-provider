// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package mcr

import (
	"fmt"
	"io"
	"os"

	iri "github.com/ironcore-dev/ironcore/iri/apis/machine/v1alpha1"
	"k8s.io/apimachinery/pkg/util/yaml"
)

func LoadMachineClasses(reader io.Reader) ([]iri.MachineClass, error) {
	var classList []iri.MachineClass
	if err := yaml.NewYAMLOrJSONDecoder(reader, 4096).Decode(&classList); err != nil {
		return nil, fmt.Errorf("unable to unmarshal machine classes: %w", err)
	}

	return classList, nil
}

func LoadMachineClassesFile(filename string) ([]iri.MachineClass, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("unable to open machine class file (%s): %w", filename, err)
	}

	return LoadMachineClasses(file)
}

func NewMachineClassRegistry(classes []iri.MachineClass) (*Mcr, error) {
	registry := Mcr{
		classes: map[string]iri.MachineClass{},
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
	classes map[string]iri.MachineClass
}

func (m *Mcr) Get(machineClassName string) (*iri.MachineClass, bool) {
	class, found := m.classes[machineClassName]
	return &class, found
}

func (m *Mcr) List() []*iri.MachineClass {
	var classes []*iri.MachineClass
	for name := range m.classes {
		class := m.classes[name]
		classes = append(classes, &class)
	}

	return classes
}
