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

package controllers

import (
	"context"
	"errors"
	"fmt"
	"github.com/go-logr/logr"
	"github.com/onmetal/libvirt-driver/pkg/api"
	"github.com/onmetal/libvirt-driver/pkg/event"
	"github.com/onmetal/libvirt-driver/pkg/store"
	"github.com/onmetal/libvirt-driver/pkg/utils"
	"k8s.io/client-go/util/workqueue"
	"slices"
	"sync"
)

const (
	MachineFinalizer = "machine"
)

type MachineReconcilerOptions struct {
}

func NewMachineReconciler(
	log logr.Logger,
	machines store.Store[*api.Machine],
	machineEvents event.Source[*api.Machine],
	opts MachineReconcilerOptions,
) (*MachineReconciler, error) {
	if machines == nil {
		return nil, fmt.Errorf("must specify machine store")
	}

	if machineEvents == nil {
		return nil, fmt.Errorf("must specify machine events")
	}

	return &MachineReconciler{
		log:           log,
		queue:         workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter()),
		machines:      machines,
		machineEvents: machineEvents,
	}, nil
}

type MachineReconciler struct {
	log   logr.Logger
	queue workqueue.RateLimitingInterface

	machines      store.Store[*api.Machine]
	machineEvents event.Source[*api.Machine]
}

func (r *MachineReconciler) Start(ctx context.Context) error {
	log := r.log

	//todo make configurable
	workerSize := 15

	imgEventReg, err := r.machineEvents.AddHandler(event.HandlerFunc[*api.Machine](func(evt event.Event[*api.Machine]) {
		r.queue.Add(evt.Object.ID)
	}))
	if err != nil {
		return err
	}
	defer func() {
		if err = r.machineEvents.RemoveHandler(imgEventReg); err != nil {
			log.Error(err, "failed to remove machine event handler")
		}
	}()

	go func() {
		<-ctx.Done()
		r.queue.ShutDown()
	}()

	var wg sync.WaitGroup
	for i := 0; i < workerSize; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for r.processNextWorkItem(ctx, log) {
			}
		}()
	}

	wg.Wait()
	return nil
}

func (r *MachineReconciler) processNextWorkItem(ctx context.Context, log logr.Logger) bool {
	item, shutdown := r.queue.Get()
	if shutdown {
		return false
	}
	defer r.queue.Done(item)

	id := item.(string)
	log = log.WithValues("machineId", id)
	ctx = logr.NewContext(ctx, log)

	if err := r.reconcileMachine(ctx, id); err != nil {
		log.Error(err, "failed to reconcile machine")
		r.queue.AddRateLimited(item)
		return true
	}

	r.queue.Forget(item)
	return true
}

func (r *MachineReconciler) reconcileMachine(ctx context.Context, id string) error {
	log := logr.FromContextOrDiscard(ctx)

	//Todo
	log.V(1).Info("Test", "id", id)

	machine, err := r.machines.Get(ctx, id)
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("failed to fetch machine from store: %w", err)
		}

		return nil
	}

	if machine.DeletedAt != nil {
		if err := r.deleteMachine(ctx, log, machine); err != nil {
			return fmt.Errorf("failed to delete machine: %w", err)
		}
		log.V(1).Info("Successfully deleted machine")
		return nil
	}

	if !slices.Contains(machine.Finalizers, MachineFinalizer) {
		machine.Finalizers = append(machine.Finalizers, MachineFinalizer)
		if _, err := r.machines.Update(ctx, machine); err != nil {
			return fmt.Errorf("failed to set finalizers: %w", err)
		}
		return nil
	}

	return nil
}

func (r *MachineReconciler) deleteMachine(ctx context.Context, log logr.Logger, machine *api.Machine) error {
	if !slices.Contains(machine.Finalizers, MachineFinalizer) {
		log.V(1).Info("machine has no finalizer: done")
		return nil
	}

	//do libvirt cleanup

	machine.Finalizers = utils.DeleteSliceElement(machine.Finalizers, MachineFinalizer)
	if _, err := r.machines.Update(ctx, machine); store.IgnoreErrNotFound(err) != nil {
		return fmt.Errorf("failed to update machine metadata: %w", err)
	}
	log.V(2).Info("Removed Finalizers")

	return nil
}
