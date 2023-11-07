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

package server

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	machinev1alpha1 "github.com/onmetal/libvirt-driver/driver/api/v1alpha1"
	"github.com/onmetal/libvirt-driver/driver/apiutils"
	"github.com/onmetal/libvirt-driver/pkg/api"
	ori "github.com/onmetal/onmetal-api/ori/apis/machine/v1alpha1"
)

func calcResources(class *ori.MachineClass) (int64, int64) {
	//Todo do some magic
	return class.Capabilities.CpuMillis, class.Capabilities.MemoryBytes
}

func (s *Server) createMachineFromORIMachine(ctx context.Context, log logr.Logger, oriMachine *ori.Machine) (*api.Machine, error) {
	log.V(2).Info("Getting libvirt machine config")

	if oriMachine == nil {
		return nil, fmt.Errorf("ori machine is nil")
	}

	class, found := s.machineClasses.Get(oriMachine.Spec.Class)
	if !found {
		return nil, fmt.Errorf("machine class '%s' not supported", oriMachine.Spec.Class)
	}
	log.V(2).Info("Validated class")

	cpu, memory := calcResources(class)

	power, err := s.getPowerStateFromOri(oriMachine.Spec.Power)
	if err != nil {
		return nil, fmt.Errorf("failed to get power state: %w", err)
	}

	var volumes []*api.VolumeSpec
	for _, oriVolume := range oriMachine.Spec.Volumes {
		var emptyDiskSpec *api.EmptyDiskSpec
		var connectionSpec *api.VolumeConnection

		if connection := oriVolume.Connection; connection != nil {
			connectionSpec = &api.VolumeConnection{
				Driver:     connection.Driver,
				Handle:     connection.Handle,
				Attributes: connection.Attributes,
				SecretData: connection.SecretData,
			}
		}

		if emptyDisk := oriVolume.EmptyDisk; emptyDisk != nil {
			emptyDiskSpec = &api.EmptyDiskSpec{
				Size: emptyDisk.SizeBytes,
			}
		}

		volumeSpec := &api.VolumeSpec{
			Name:       oriVolume.Name,
			Device:     oriVolume.Device,
			EmptyDisk:  emptyDiskSpec,
			Connection: connectionSpec,
		}

		if _, err := s.volumePlugins.FindPluginBySpec(volumeSpec); err != nil {
			return nil, fmt.Errorf("failed to find volume plugin: %w", err)
		}
		volumes = append(volumes, volumeSpec)
	}

	machine := &api.Machine{
		Metadata: api.Metadata{
			ID: s.idGen.Generate(),
		},
		Spec: api.MachineSpec{
			Power:       power,
			CpuMillis:   cpu,
			MemoryBytes: memory,
			Volumes:     volumes,
		},
	}

	if err := apiutils.SetObjectMetadata(machine, oriMachine.Metadata); err != nil {
		return nil, fmt.Errorf("failed to set metadata: %w", err)
	}
	apiutils.SetClassLabel(machine, oriMachine.Spec.Class)
	apiutils.SetManagerLabel(machine, machinev1alpha1.MachineManager)

	apiMachine, err := s.machineStore.Create(ctx, machine)
	if err != nil {
		return nil, fmt.Errorf("failed to create machine: %w", err)
	}

	return apiMachine, nil
}

func (s *Server) CreateMachine(ctx context.Context, req *ori.CreateMachineRequest) (res *ori.CreateMachineResponse, retErr error) {
	log := s.loggerFrom(ctx)

	log.V(1).Info("Creating machine from ori machine")
	machine, err := s.createMachineFromORIMachine(ctx, log, req.Machine)
	if err != nil {
		return nil, fmt.Errorf("unable to get libvirt machine config: %w", err)
	}

	log.V(1).Info("Converting machine to ori machine")
	oriMachine, err := s.convertMachineToOriMachine(ctx, log, machine)
	if err != nil {
		return nil, fmt.Errorf("unable to convert machine: %w", err)
	}

	return &ori.CreateMachineResponse{
		Machine: oriMachine,
	}, nil
}
