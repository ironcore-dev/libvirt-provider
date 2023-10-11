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
	volumeplugin "github.com/onmetal/libvirt-driver/pkg/plugins/volume"
	"github.com/onmetal/libvirt-driver/pkg/store"
	"github.com/onmetal/libvirt-driver/pkg/utils"
	"k8s.io/client-go/util/workqueue"
	"slices"
	"sync"
)

const (
	VolumeFinalizer = "volume"
)

type VolumeReconcilerOptions struct {
}

func NewVolumeReconciler(
	log logr.Logger,
	volumes store.Store[*api.Volume],
	volumeEvents event.Source[*api.Volume],
	opts VolumeReconcilerOptions,
) (*VolumeReconciler, error) {
	if volumes == nil {
		return nil, fmt.Errorf("must specify volume store")
	}

	if volumeEvents == nil {
		return nil, fmt.Errorf("must specify volume events")
	}

	return &VolumeReconciler{
		log:          log,
		queue:        workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter()),
		volumeEvents: volumeEvents,
		volumes:      volumes,
	}, nil
}

type VolumeReconciler struct {
	log   logr.Logger
	queue workqueue.RateLimitingInterface

	volumePlugins *volumeplugin.PluginManager

	volumes      store.Store[*api.Volume]
	volumeEvents event.Source[*api.Volume]
}

func (r *VolumeReconciler) Start(ctx context.Context) error {
	log := r.log

	//todo make configurable
	workerSize := 15

	volumeEventReg, err := r.volumeEvents.AddHandler(event.HandlerFunc[*api.Volume](func(evt event.Event[*api.Volume]) {
		r.queue.Add(evt.Object.ID)
	}))
	if err != nil {
		return err
	}
	defer func() {
		if err = r.volumeEvents.RemoveHandler(volumeEventReg); err != nil {
			log.Error(err, "failed to remove volume event handler")
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

func (r *VolumeReconciler) processNextWorkItem(ctx context.Context, log logr.Logger) bool {
	item, shutdown := r.queue.Get()
	if shutdown {
		return false
	}
	defer r.queue.Done(item)

	id := item.(string)
	log = log.WithValues("volumeId", id)
	ctx = logr.NewContext(ctx, log)

	if err := r.reconcileVolume(ctx, id); err != nil {
		log.Error(err, "failed to reconcile volume")
		r.queue.AddRateLimited(item)
		return true
	}

	r.queue.Forget(item)
	return true
}

func (r *VolumeReconciler) reconcileVolume(ctx context.Context, id string) error {
	log := logr.FromContextOrDiscard(ctx)

	log.V(2).Info("Getting volume from store", "id", id)
	volume, err := r.volumes.Get(ctx, id)
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("failed to fetch volume from store: %w", err)
		}

		return nil
	}

	if volume.DeletedAt != nil {
		if err := r.deleteVolume(ctx, log, volume); err != nil {
			return fmt.Errorf("failed to delete volume: %w", err)
		}
		log.V(1).Info("Successfully deleted volume")
		return nil
	}

	if !slices.Contains(volume.Finalizers, VolumeFinalizer) {
		volume.Finalizers = append(volume.Finalizers, VolumeFinalizer)
		if _, err := r.volumes.Update(ctx, volume); err != nil {
			return fmt.Errorf("failed to set finalizers: %w", err)
		}
		return nil
	}

	plugin, err := r.volumePlugins.FindPluginByName(string(volume.Spec.Provider))
	if err != nil {
		return fmt.Errorf("failed to find volume plugin %s", volume.Spec.Provider)
	}

	status, err := plugin.Apply(ctx, volume)
	if err != nil {
		return fmt.Errorf("failed to apply volume: %w", err)
	}

	volume.Status = *status

	return nil
}

func (r *VolumeReconciler) deleteVolume(ctx context.Context, log logr.Logger, volume *api.Volume) error {
	if !slices.Contains(volume.Finalizers, VolumeFinalizer) {
		log.V(1).Info("volume has no finalizer: done")
		return nil
	}

	plugin, err := r.volumePlugins.FindPluginByName(string(volume.Spec.Provider))
	if err != nil {
		return fmt.Errorf("failed to find volume plugin %s", volume.Spec.Provider)
	}

	if err := plugin.Delete(ctx, volume.ID); err != nil {
		return fmt.Errorf("failed to delete volume: %w", err)
	}

	volume.Finalizers = utils.DeleteSliceElement(volume.Finalizers, VolumeFinalizer)
	if _, err := r.volumes.Update(ctx, volume); store.IgnoreErrNotFound(err) != nil {
		return fmt.Errorf("failed to update volume metadata: %w", err)
	}
	log.V(2).Info("Removed Finalizers")

	return nil
}
