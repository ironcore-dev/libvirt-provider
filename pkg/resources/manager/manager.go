// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strconv"
	"sync"

	core "github.com/ironcore-dev/ironcore/api/core/v1alpha1"
	iri "github.com/ironcore-dev/ironcore/iri/apis/machine/v1alpha1"
	"github.com/ironcore-dev/libvirt-provider/pkg/api"
	"github.com/ironcore-dev/libvirt-provider/pkg/resources/sources"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/yaml"

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

	ErrResourceUnsupported = errors.New("resource isn't supported")

	ErrMachineClassMissing = errors.New("machine class is missing")

	ErrCommonResources = errors.New("common resources managed by different sources")

	ErrVMLimitReached = errors.New("vm limit is already reached")
)

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
	machineClasses []*MachineClass

	machineclassesFile string

	// sources is register of all added sources
	sources map[string]Source

	registredResources map[core.ResourceName]Source

	// mx allow change internal state only one gouroutines
	mx sync.Mutex

	// intialized protects manager before double initialization
	initialized bool

	// operationError optimize execution of allocate and deallocate function
	// and it serves as protection for calling function before initialization.
	operationError error

	// availableVMSlots signifies the number of VMs that can still be created on the host
	availableVMSlots int64

	// maxVMsLimit signifies maximum number of VMs allowed on host
	maxVMsLimit uint64
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

func (r *resourceManager) setLogger(logger logr.Logger) error {
	r.mx.Lock()
	defer r.mx.Unlock()

	if r.initialized {
		return ErrManagerAlreadyInitialized
	}

	r.log = logger.WithName("resource-manager")
	return nil
}

func (r *resourceManager) setMachineClassesFilename(filename string) error {
	if r.initialized {
		return ErrManagerAlreadyInitialized
	}

	r.machineclassesFile = filename

	return nil
}

func (r *resourceManager) setVMLimit(maxVMsLimit uint64) error {
	if r.initialized {
		return ErrManagerAlreadyInitialized
	}

	r.maxVMsLimit = maxVMsLimit

	return nil
}

func (r *resourceManager) initMachineClasses() error {
	classes, err := loadMachineClassesFile(r.machineclassesFile)
	if err != nil {
		return err
	}

MAIN:
	for _, class := range classes {
		cpu, ok := class.Capabilities[core.ResourceCPU]
		if !ok {
			return fmt.Errorf("required resource %s is missing in machine class file", core.ResourceCPU)
		}

		mem, ok := class.Capabilities[core.ResourceMemory]
		if !ok {
			return fmt.Errorf("required resource %s is missing in machine class file", core.ResourceMemory)
		}

		// it is for minimize creation operations in status call
		class.iriClass = &iri.MachineClass{
			Name: class.Name,
			Capabilities: &iri.MachineClassCapabilities{
				CpuMillis:   cpu.MilliValue(),
				MemoryBytes: mem.Value(),
			},
		}

		for key := range class.Capabilities {
			_, ok := r.registredResources[key]
			if !ok {
				r.log.Error(fmt.Errorf("missing source for resource %s: %w", key, ErrResourceUnsupported), fmt.Sprintf("machine class %s will be ignore", class.Name))
				continue MAIN
			}
		}

		r.machineClasses = append(r.machineClasses, class)

		// rounding base on provider configuration
		err := r.modifyResources(class.Capabilities)
		if err != nil {
			return err
		}

		// TODO: do we need still this?
		err = r.calculateMachineClassQuantity(class)
		if err != nil {
			return err
		}
	}

	sort.Slice(r.machineClasses, func(i, j int) bool {
		return r.machineClasses[i].Name < r.machineClasses[j].Name
	})

	return nil
}

func (r *resourceManager) initialize(ctx context.Context, machines []*api.Machine) error {
	r.mx.Lock()
	defer r.mx.Unlock()

	if len(r.sources) == 0 {
		return ErrManagerSourcesMissing
	}

	if r.initialized {
		return ErrManagerAlreadyInitialized
	}

	// reinit after error isn't possible
	r.initialized = true

	r.ctx = ctx

	totalExistingVMCount := uint64(len(machines))
	if r.maxVMsLimit != 0 && totalExistingVMCount >= r.maxVMsLimit {
		r.log.Info("VM limit is already reached", "Limit", r.maxVMsLimit, "Existing count", totalExistingVMCount)
	}
	r.availableVMSlots = int64(r.maxVMsLimit - totalExistingVMCount)

	for _, s := range r.sources {
		resources, err := s.Init(r.ctx)
		if err != nil {
			return err
		}

		for _, value := range resources.UnsortedList() {
			conflictedSource, ok := r.registredResources[value]
			if ok {
				return fmt.Errorf("%w: sources %s and %s mnaged same resource %s", ErrCommonResources, s.GetName(), conflictedSource.GetName(), value)
			}

			r.registredResources[value] = s
		}
	}

	r.log.Info("Initialized resources: " + r.convertResourcesToString(r.getAvailableResources()))

	// Allocating resources for pre-existing machines in store
	for _, machine := range machines {
		for _, s := range r.sources {
			_, err := s.Allocate(machine.Spec.Resources.DeepCopy())
			if err != nil {
				return err
			}
		}
	}

	err := r.initMachineClasses()
	if err != nil {
		return err
	}

	r.log.Info("Available VM slots:" + r.getAvailableVMSlotsAsString())
	r.log.Info("Available resources: " + r.convertResourcesToString(r.getAvailableResources()))
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

	if r.maxVMsLimit != 0 && r.availableVMSlots <= 0 {
		return ErrVMLimitReached
	}

	totalAllocatedRes := core.ResourceList{}
	var allocatedRes core.ResourceList
	for key := range requiredResources {
		s, ok := r.registredResources[key]
		if !ok {
			return fmt.Errorf("failed to find source for resource %s: %w", key, ErrResourceUnsupported)
		}

		allocatedRes, err = s.Allocate(requiredResources)
		if err != nil {
			r.deallocateUnassignResources(totalAllocatedRes)
			return err
		}

		mergeResourceLists(totalAllocatedRes, allocatedRes)
	}

	machine.Spec.Resources = totalAllocatedRes

	if r.numaScheduler != nil {
		cpuQuantity := requiredResources[core.ResourceCPU]
		err = r.numaScheduler.Pin(uint(cpuQuantity.Value()/1000), machine)
		if err != nil {
			return err
		}
	}

	r.availableVMSlots -= 1

	// error cannot ocurre here
	_ = r.updateMachineClassAvailable()

	r.printAvailableResources("allocation")

	return nil
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

	if r.numaScheduler != nil {
		err = r.numaScheduler.Unpin(machine)
		if err != nil {
			return err
		}
	}
	for key := range deallocateResources {
		s := r.registredResources[key]
		resourceNames := s.Deallocate(deallocateResources)

		for _, resource := range resourceNames {
			// TODO: we have to optimize this
			delete(deallocateResources, key)
			delete(machine.Spec.Resources, resource)
		}
	}

	r.availableVMSlots += 1

	// error cannot occure here
	_ = r.updateMachineClassAvailable()

	r.printAvailableResources("deallocation")

	return nil
}

func (r *resourceManager) deallocateUnassignResources(resources core.ResourceList) {
	for key := range resources {
		// if resource is allocated, source has to exist
		s := r.registredResources[key]
		_ = s.Deallocate(resources)
	}
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
		i := *class.iriClass
		classStatus := &iri.MachineClassStatus{
			MachineClass: &i,
			Quantity:     class.available,
		}

		status[index] = classStatus
	}

	return status
}

func (r *resourceManager) calculateMachineClassQuantity(class *MachineClass) error {
	var count int64 = math.MaxInt64

	classResources := class.Capabilities.DeepCopy()
	for key := range classResources {
		s, ok := r.registredResources[key]
		if !ok {
			return fmt.Errorf("failed to find source for resource %s: %w", key, ErrManagerSourcesMissing)
		}
		sourceCount := s.CalculateMachineClassQuantity(classResources)
		if sourceCount == 0 {
			count = 0
			break
		}

		if sourceCount == sources.QuantityCountIgnore {
			continue
		}

		if count > sourceCount {
			count = sourceCount
		}
	}

	if r.maxVMsLimit != 0 && count > r.availableVMSlots {
		count = r.availableVMSlots
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

func (r *resourceManager) getMachineClass(name string) (*MachineClass, error) {
	for _, class := range r.machineClasses {
		if class.Name == name {
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

	r.log.V(traceLevel).Info("Available VM slots after " + operation + ": " + r.getAvailableVMSlotsAsString())
	r.log.V(traceLevel).Info("Available resources after " + operation + ": " + r.convertResourcesToString(r.getAvailableResources()))
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
		result += class.Name + ": " + strconv.FormatInt(class.available, 10) + sep
	}

	return removeSeparatorFromEnd(result)
}

func (r *resourceManager) getAvailableVMSlotsAsString() string {
	count := r.availableVMSlots

	if count < 0 {
		count = 0
	}

	return strconv.FormatInt(count, 10)
}

func (r *resourceManager) getAvailableResources() core.ResourceList {
	resourceList := make(core.ResourceList)

	for _, s := range r.sources {
		list := s.GetAvailableResources()
		mergeResourceLists(resourceList, list)
	}
	return resourceList
}

func (r *resourceManager) getIRIMachineClasses() []iri.MachineClass {
	iriClasses := make([]iri.MachineClass, 0, len(r.machineClasses))
	for _, class := range r.machineClasses {
		iriClasses = append(iriClasses, *class.iriClass)
	}

	return iriClasses
}

func mergeResourceLists(dst, src core.ResourceList) {
	for k, v := range src {
		dst[k] = v
	}
}

// reset internal state of manager and allow reinit
// Use it for unit test only.
func (r *resourceManager) reset() {
	r.ctx = nil
	r.log = ctrl.Log
	r.machineClasses = nil
	r.operationError = ErrManagerNotInitialized
	r.sources = map[string]Source{}
	r.machineclassesFile = ""
	r.initialized = false
	r.registredResources = map[core.ResourceName]Source{}
}

func removeSeparatorFromEnd(in string) string {
	if len(in) <= len(sep) {
		return ""
	}

	return in[:len(in)-len(sep)]
}

func loadMachineClasses(reader io.Reader) ([]*MachineClass, error) {
	var classList []*MachineClass
	if err := yaml.NewYAMLOrJSONDecoder(reader, 4096).Decode(&classList); err != nil {
		return nil, fmt.Errorf("unable to unmarshal machine classes: %w", err)
	}

	return classList, nil
}

func loadMachineClassesFile(filename string) ([]*MachineClass, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("unable to open machine class file (%s): %w", filename, err)
	}

	return loadMachineClasses(file)
}
