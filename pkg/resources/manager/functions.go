// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	core "github.com/ironcore-dev/ironcore/api/core/v1alpha1"
	iri "github.com/ironcore-dev/ironcore/iri/apis/machine/v1alpha1"
	"github.com/ironcore-dev/libvirt-provider/pkg/api"
	"github.com/ironcore-dev/libvirt-provider/pkg/resources/sources"
	"github.com/ironcore-dev/libvirt-provider/pkg/sgx"
)

// AddSource just registers source into manager
func AddSource(source Source) error {
	return mng.addSource(source)
}

// Allocate reserve resources base on machine class.
// Allocated resources are saved into machine specification.
// All resources has to allocated, partially allocation isn't supported.
func Allocate(machine *api.Machine, requiredResources core.ResourceList) error {
	if len(requiredResources) == 0 {
		return ErrResourcesEmpty
	}

	return mng.allocate(machine, requiredResources)
}

// Deallocate free all resources from machine class.
// Deallocated resources are deleted from machine specification.
func Deallocate(machine *api.Machine, deallocateResources core.ResourceList) error {
	if len(deallocateResources) == 0 {
		return ErrResourcesEmpty
	}

	if !HasMachineAllocatedResources(machine) {
		return nil
	}

	return mng.deallocate(machine, deallocateResources)
}

// SetLogger sets logger for internal logging.
// It will add resource-manager into name of logger
func SetLogger(logger logr.Logger) error {
	return mng.setLogger(logger)
}

// SetMachineClasses just registers filename with machineclasses which are load during initializating
func SetMachineClassesFilename(filename string) error {
	return mng.setMachineClassesFilename(filename)
}

// GetIRIMAchineClasses will return machineClasses of resource manager as IRI Machine Classes
func GetIRIMachineClasses() []iri.MachineClass {
	return mng.getIRIMachineClasses()
}

// SetVMLimit just registers maximum limit for VMs
func SetVMLimit(maxVMsLimit uint64) error {
	return mng.setVMLimit(maxVMsLimit)
}

// GetMachineClassStatus return status of machineclasses with current quantity
func GetMachineClassStatus() []*iri.MachineClassStatus {
	return mng.getAvailableMachineClasses()
}

// Initialize inits resource mng.
// Initialize can be call just one time.
// Before Initialize you can call SetMachineClasses, SetLogger, AddSource functions.
// It will calculate available resources during start of app.
// After Initialize you can call Allocate and Deallocate functions.
func Initialize(ctx context.Context, listMachines func(context.Context) ([]*api.Machine, error)) error {
	if listMachines == nil {
		return ErrManagerListFuncInvalid
	}

	machines, err := listMachines(ctx)
	if err != nil {
		return err
	}

	return mng.initialize(ctx, machines)
}

func HasMachineAllocatedResources(machine *api.Machine) bool {
	return len(machine.Spec.Resources) != 0
}

func GetSource(name string, options sources.Options) (Source, error) {
	switch name {
	case sources.SourceMemory:
		return sources.NewSourceMemory(options), nil
	case sources.SourceCPU:
		return sources.NewSourceCPU(options), nil
	case sources.SourceHugepages:
		return sources.NewSourceHugepages(options), nil
	case sgx.SourceSGX:
		return sgx.NewSourceSGX(options), nil
	default:
		return nil, fmt.Errorf("unsupported source %s", name)
	}
}

func GetSourcesAvailable() []string {
	return []string{sources.SourceCPU, sources.SourceMemory, sources.SourceHugepages, sgx.SourceSGX}
}

func GetMachineClassRequiredResources(name string) (core.ResourceList, error) {
	class, err := mng.getMachineClass(name)
	if err != nil {
		return nil, err
	}

	return class.Capabilities.DeepCopy(), nil
}

func ValidateOptions(options sources.Options) error {
	// To handle the limitations of floating-point arithmetic, where small rounding errors can occur
	// due to the finite precision of floating-point numbers.
	if options.OvercommitVCPU < 1e-9 {
		return errors.New("overcommitVCPU cannot be zero or negative")
	}

	var hasMemory, hasHugepages bool
	for _, source := range options.Sources {
		if source == sources.SourceMemory {
			hasMemory = true
		}
		if source == sources.SourceHugepages {
			hasHugepages = true
		}
	}

	if options.ReservedMemorySize != 0 && !hasMemory {
		return fmt.Errorf("reserved memory size can only be set with %s source", sources.SourceMemory)
	}

	if options.BlockedHugepages != 0 && !hasHugepages {
		return fmt.Errorf("blocked hugepages can only be set with %s source", sources.SourceHugepages)
	}

	return nil
}
