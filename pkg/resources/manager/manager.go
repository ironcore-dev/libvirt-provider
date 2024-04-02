// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"sync"

	core "github.com/ironcore-dev/ironcore/api/core/v1alpha1"
	iri "github.com/ironcore-dev/ironcore/iri/apis/machine/v1alpha1"
	"github.com/ironcore-dev/libvirt-provider/pkg/api"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
)

const sep = ", "

var (
	ErrManagerAlreadyInitialized = errors.New("resource manager is already initialized")
	ErrManagerNotInitialized     = errors.New("resource manager isn't initialized")
	ErrManagerSourcesMissing     = errors.New("any source wasn't registered")
	ErrManagerListFuncInvalid    = errors.New("invalid pointer to list machine function")

	ErrResourcesNotInitialized = errors.New("resources aren't initialized")
	ErrResourcesEmpty          = errors.New("resources cannot be empty")

	ErrResourceNotAvailable      = errors.New("not enough available resources")
	ErrResourceUnsupported       = errors.New("resource isn't supported")
	ErrResourceAlreadyRegistered = errors.New("resource is already registered")
	ErrResourceNegativeQuantity  = errors.New("resource quantity is negative")

	ErrMachineClassMissing = errors.New("machine class is missing")

	ErrMachineHasAllocatedResources = errors.New("machine has already allocated resources")
)

type totalResourcesTuneFunc func(core.ResourceList) error

func init() {
	mng.reset()
}

// unexported manager structure, it contains state and main logic
var mng resourceManager

type resourceManager struct {
	// context serves primary for optimize shutdown and it can be use in sources
	ctx context.Context
	// resourceManager logger with resource-manager in name
	log logr.Logger
	// numaScheduler serves for planning pinning numa zones/cpu
	numaScheduler NumaScheduler
	// internal implementation of machineclasses with quantity/availibility
	machineClasses []*machineclass

	// it will replaced with machineClasses and
	// it is temporary storage for compatibility with build pattern
	tmpIRIMachineClasses []*iri.MachineClass

	// sources is register of all added sources
	sources map[core.ResourceName]Source

	// resoruceAvailable keep current state of available resources
	resourcesAvailable core.ResourceList

	// mx allow change internal state only one gouroutines
	mx sync.Mutex

	// intialized protects manager before double initialization
	initialized bool

	// operationError optimize execution of allocate and deallocate function
	// and it serves as protection for calling function before initialization.
	operationError error

	// totalResourcesTuners tunes the availability of total resources for the vms
	totalResourcesTuners []totalResourcesTuneFunc
}

func (r *resourceManager) addSource(source Source) error {
	r.mx.Lock()
	defer r.mx.Unlock()

	if r.initialized {
		return ErrManagerAlreadyInitialized
	}

	r.sources[source.GetName()] = source

	return nil
}

func (r *resourceManager) addTotalResourcesTuner(totalResourcesTuneFunc totalResourcesTuneFunc) error {
	r.mx.Lock()
	defer r.mx.Unlock()

	if r.initialized {
		return ErrManagerAlreadyInitialized
	}

	r.totalResourcesTuners = append(r.totalResourcesTuners, totalResourcesTuneFunc)

	return nil
}

func (r *resourceManager) setLogger(logger logr.Logger) error {
	r.mx.Lock()
	defer r.mx.Unlock()

	if r.initialized {
		return ErrManagerAlreadyInitialized
	}

	r.log = logger.WithName("resource-manager")
	return nil
}

func (r *resourceManager) setMachineClasses(classes []*iri.MachineClass) error {
	if r.initialized {
		return ErrManagerAlreadyInitialized
	}

	r.tmpIRIMachineClasses = classes

	return nil
}

func (r *resourceManager) loadTotalResources() error {
	if len(r.sources) == 0 {
		return ErrManagerSourcesMissing
	}

	for sourceName, source := range r.sources {
		r.log.V(1).Info("loading total resources from source " + string(sourceName))
		resources, err := source.GetTotalResources(r.ctx)
		if err != nil {
			return err
		}
		for name, quantity := range resources {
			_, locErr := getResource(name, r.resourcesAvailable)
			if locErr == nil {
				return fmt.Errorf("resource %s cannot be register: %w", name, ErrResourceAlreadyRegistered)
			}

			r.resourcesAvailable[name] = quantity
		}
	}

	for _, tune := range r.totalResourcesTuners {
		if err := tune(r.resourcesAvailable); err != nil {
			return err
		}
	}

	r.log.Info("Host total resources: " + r.convertResourcesToString(r.resourcesAvailable))

	return nil
}

func (r *resourceManager) calculateAvailableResources(machines []*api.Machine) error {
	for _, machine := range machines {
		newAvailableResources, err := r.preallocateAvailableResources(machine.Spec.Resources)
		if err != nil {
			return err
		}

		r.resourcesAvailable = newAvailableResources
	}

	return nil
}

func (r *resourceManager) createMachineClasses() error {
	r.machineClasses = make([]*machineclass, len(r.tmpIRIMachineClasses))
	for index, class := range r.tmpIRIMachineClasses {
		resourceManagerClass := &machineclass{
			// break reference
			name:         class.GetName(),
			capabilities: iri.MachineClassCapabilities{CpuMillis: class.Capabilities.CpuMillis, MemoryBytes: class.Capabilities.MemoryBytes},
			resources: core.ResourceList{
				core.ResourceCPU:    *resource.NewQuantity(class.Capabilities.CpuMillis, resource.DecimalSI),
				core.ResourceMemory: *resource.NewQuantity(class.Capabilities.MemoryBytes, resource.BinarySI),
			},
		}

		// rounding base on provider configuration
		err := r.modifyResources(resourceManagerClass.resources)
		if err != nil {
			return err
		}

		err = r.calculateMachineClassQuantity(resourceManagerClass)
		if err != nil {
			return err
		}

		r.machineClasses[index] = resourceManagerClass
	}

	sort.Slice(r.machineClasses, func(i, j int) bool {
		return r.machineClasses[i].name < r.machineClasses[j].name
	})

	r.tmpIRIMachineClasses = nil
	return nil
}

func (r *resourceManager) initialize(ctx context.Context, machines []*api.Machine) error {
	r.mx.Lock()
	defer r.mx.Unlock()

	if r.initialized {
		return ErrManagerAlreadyInitialized
	}

	// reinit after error isn't possible
	r.initialized = true

	r.ctx = ctx

	err := r.loadTotalResources()
	if err != nil {
		return err
	}

	//TODO implement "limiters"

	err = r.calculateAvailableResources(machines)
	if err != nil {
		return err
	}

	err = r.createMachineClasses()
	if err != nil {
		return err
	}

	r.log.Info("Available resources: " + r.convertResourcesToString(r.resourcesAvailable))
	r.log.Info("Machine classes availibility: " + r.getMachineClassAvailibilityAsString())
	r.operationError = nil

	return nil
}

func (r *resourceManager) allocate(machine *api.Machine, requiredResources core.ResourceList) error {
	r.mx.Lock()
	defer r.mx.Unlock()

	if r.operationError != nil {
		return r.operationError
	}

	err := r.checkContext()
	if err != nil {
		return err
	}

	newAvailableResources, err := r.preallocateAvailableResources(requiredResources)
	if err != nil {
		return err
	}

	if r.numaScheduler != nil {
		cpuQuantity := requiredResources[core.ResourceCPU]
		err = r.numaScheduler.Pin(uint(cpuQuantity.Value()/1000), machine)
		if err != nil {
			return err
		}
	}

	assignResourcesIntoMachine(&machine.Spec, requiredResources)

	r.resourcesAvailable = newAvailableResources

	// error cannot ocurre here
	_ = r.updateMachineClassAvailable()

	r.printAvailableResources("allocation")

	return nil
}

// preallocateAvailableResources will recalculate available resources and return new state
func (r *resourceManager) preallocateAvailableResources(resources core.ResourceList) (core.ResourceList, error) {
	newAvailableResources := r.resourcesAvailable.DeepCopy()
	for key, resource := range resources {
		available, err := getResource(key, newAvailableResources)
		if err != nil {
			return nil, err
		}

		available.Sub(resource)
		if available.Sign() == -1 {
			return nil, ErrResourceNotAvailable
		}
		newAvailableResources[key] = *available
	}

	return newAvailableResources, nil
}

func (r *resourceManager) deallocate(machine *api.Machine, deallocateResources core.ResourceList) error {
	r.mx.Lock()
	defer r.mx.Unlock()

	if r.operationError != nil {
		return r.operationError
	}

	err := r.checkContext()
	if err != nil {
		return err
	}

	newResources := r.resourcesAvailable
	for key, resource := range deallocateResources {
		available, err := getResource(key, newResources)
		if err != nil {
			return err
		}

		available.Add(resource)
		newResources[key] = *available
	}

	if r.numaScheduler != nil {
		err = r.numaScheduler.Unpin(machine)
		if err != nil {
			return err
		}
	}

	err = deassignResourcesFromMachine(&machine.Spec, deallocateResources)
	if err != nil {
		return err
	}

	r.resourcesAvailable = newResources

	// error cannot occure here
	_ = r.updateMachineClassAvailable()

	r.printAvailableResources("deallocation")

	return nil
}

// updateMachineClassAvailable is updating count of available machines for all machine classes
func (r *resourceManager) updateMachineClassAvailable() error {
	for _, class := range r.machineClasses {
		err := r.calculateMachineClassQuantity(class)
		if err != nil {
			return err
		}
	}

	return nil
}

func (r *resourceManager) getAvailableMachineClasses() []*iri.MachineClassStatus {
	r.mx.Lock()
	defer r.mx.Unlock()

	status := make([]*iri.MachineClassStatus, len(r.machineClasses))
	for index, class := range r.machineClasses {
		// break references between components
		cap := class.capabilities
		classStatus := &iri.MachineClassStatus{
			MachineClass: &iri.MachineClass{
				Name:         class.name,
				Capabilities: &cap,
			},
			Quantity: class.available,
		}

		status[index] = classStatus
	}

	return status
}

func (r *resourceManager) calculateMachineClassQuantity(class *machineclass) error {
	var count int64 = math.MaxInt64
	for key, classQuantity := range class.resources {
		available, err := getResource(key, r.resourcesAvailable)
		if err != nil {
			return err
		}

		newCount := int64(math.Floor(float64(available.Value()) / float64(classQuantity.Value())))
		if newCount < count {
			count = newCount
		}
	}

	if count < 0 {
		count = 0
	}

	class.available = count

	return nil
}

func (r *resourceManager) modifyResources(resources core.ResourceList) error {
	for name, source := range r.sources {
		err := source.Modify(resources)
		if err != nil {
			return fmt.Errorf("source %s couldn't modified resources: %w", name, err)
		}
	}

	return nil
}

// checkContext will return immediately error without additional allocate/deallocate operation
// when parent context is closed. It optimizes time spend in mutex
func (r *resourceManager) checkContext() error {
	var err error
	if r.ctx.Err() != nil {
		err = fmt.Errorf("context error: %w", r.ctx.Err())
		r.operationError = err
	}

	return err
}

func getResource(name core.ResourceName, resources core.ResourceList) (*resource.Quantity, error) {
	quantity, ok := resources[name]
	if !ok {
		return nil, fmt.Errorf("failed to get %s resource: %w", name, ErrResourceUnsupported)
	}

	return &quantity, nil
}

func (r *resourceManager) getMachineClass(name string) (*machineclass, error) {
	for _, class := range r.machineClasses {
		if class.name == name {
			return class, nil
		}
	}

	return nil, ErrMachineClassMissing
}

// printAvailableResources is helpful function for avoid unnecessary operations
func (r *resourceManager) printAvailableResources(operation string) {
	const traceLevel = 2
	if !r.log.V(traceLevel).Enabled() {
		return
	}

	r.log.V(traceLevel).Info("Available resources after " + operation + ": " + r.convertResourcesToString(r.resourcesAvailable))
	r.log.V(traceLevel).Info("Machineclasses availibility: " + r.getMachineClassAvailibilityAsString())
}

func (r *resourceManager) convertResourcesToString(resources core.ResourceList) string {
	var result string
	type res struct {
		name     string
		quantity resource.Quantity
	}

	arr := make([]res, 0, len(resources))

	for key, quantity := range resources {
		arr = append(arr, res{name: string(key), quantity: quantity})
	}

	sort.Slice(arr, func(i, j int) bool {
		return arr[i].name < arr[j].name
	})

	for index := range arr {
		result += arr[index].name + ": " + arr[index].quantity.String() + sep
	}

	return removeSeparatorFromEnd(result)
}

func (r *resourceManager) getMachineClassAvailibilityAsString() string {
	var result string
	for _, class := range r.machineClasses {
		result += class.name + ": " + strconv.FormatInt(class.available, 10) + sep
	}

	return removeSeparatorFromEnd(result)
}

// reset internal state of manager and allow reinit
// Use it for unit test only.
func (r *resourceManager) reset() {
	r.ctx = nil
	r.log = ctrl.Log
	r.machineClasses = nil
	r.numaScheduler = nil
	r.operationError = ErrManagerNotInitialized
	r.sources = map[core.ResourceName]Source{}
	r.resourcesAvailable = core.ResourceList{}
	r.tmpIRIMachineClasses = nil
	r.initialized = false
}

func removeSeparatorFromEnd(in string) string {
	if len(in) <= len(sep) {
		return ""
	}

	return in[:len(in)-len(sep)]
}

func assignResourcesIntoMachine(machineSpec *api.MachineSpec, requiredResources core.ResourceList) {
	if len(machineSpec.Resources) == 0 {
		machineSpec.Resources = requiredResources.DeepCopy()
		return
	}

	for key, resource := range requiredResources {
		quantity, ok := machineSpec.Resources[key]
		if !ok {
			machineSpec.Resources[key] = resource
			continue
		}

		// quantity use add function for support resources which can be extend during run (storage, network card)
		// WARN: this logic can be problemtic in future, if machine can change machineclass during running
		quantity.Add(resource)
		machineSpec.Resources[key] = quantity
	}
}

func deassignResourcesFromMachine(machineSpec *api.MachineSpec, deallocatedResources core.ResourceList) error {
	if len(machineSpec.Resources) == 0 {
		return ErrResourcesNotInitialized
	}

	machineResources := machineSpec.Resources.DeepCopy()
	for key, resource := range deallocatedResources {
		quantity, err := getResource(key, machineResources)
		if err != nil {
			return fmt.Errorf("failed to deassign resource %s: %w", key, err)
		}

		if quantity.Sign() == -1 {
			return fmt.Errorf("failed to deassign resource %s: %w", key, ErrResourceNegativeQuantity)
		}

		quantity.Sub(resource)
		if quantity.IsZero() {
			delete(machineResources, key)
		} else {
			machineResources[key] = *quantity
		}
	}

	if len(machineResources) == 0 {
		machineResources = nil
	}

	machineSpec.Resources = machineResources

	return nil
}
