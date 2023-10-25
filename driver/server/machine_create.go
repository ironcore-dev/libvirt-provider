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

type LibvirtMachineConfig struct {
	Machine *api.Machine
	Volumes []*api.Volume
	Nics    []*api.NetworkInterface
}

type AggregateMachine struct {
	Machine *api.Machine
	Volumes []*api.Volume
	Nics    []*api.NetworkInterface
}

func calcResources(class *ori.MachineClass) (int64, int64) {
	//Todo do some magic
	return class.Capabilities.CpuMillis, class.Capabilities.MemoryBytes
}

func (s *Server) getLibvirtMachineConfig(ctx context.Context, log logr.Logger, oriMachine *ori.Machine) (*LibvirtMachineConfig, error) {
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

	machine := &api.Machine{
		Metadata: api.Metadata{
			ID: s.idGen.Generate(),
		},
		Spec: api.MachineSpec{
			Power:       power,
			CpuMillis:   cpu,
			MemoryBytes: memory,
		},
	}

	var volumes []*api.Volume
	for _, oriVolume := range oriMachine.Spec.Volumes {
		plugin, err := s.volumePlugins.FindPluginBySpec(oriVolume)
		if err != nil {
			log.V(1).Error(err, "failed to find volume plugin")
			continue
		}
		volumeSpec, err := plugin.Prepare(oriVolume)
		if err != nil {
			log.V(1).Error(err, "failed to apply volume")
			continue
		}

		volume := &api.Volume{
			Metadata: api.Metadata{
				ID: s.idGen.Generate(),
			},
			Spec: *volumeSpec,
		}
		apiutils.SetMachineRefLabel(volume, machine.ID)

		volumes = append(volumes, volume)
		machine.Spec.Volumes = append(machine.Spec.Volumes, volume.ID)
	}

	if err := apiutils.SetObjectMetadata(machine, oriMachine.Metadata); err != nil {
		return nil, fmt.Errorf("failed to set metadata: %w", err)
	}
	apiutils.SetClassLabel(machine, oriMachine.Spec.Class)
	apiutils.SetManagerLabel(machine, machinev1alpha1.MachineManager)

	return &LibvirtMachineConfig{
		Machine: machine,
		Volumes: volumes,
	}, nil
}

func (s *Server) createLibvirtMachine(ctx context.Context, log logr.Logger, cfg *LibvirtMachineConfig) (*AggregateMachine, error) {
	aggregated := &AggregateMachine{}
	for _, volumeCfg := range cfg.Volumes {
		volume, err := s.volumeStore.Create(ctx, volumeCfg)
		if err != nil {
			continue
		}
		aggregated.Volumes = append(aggregated.Volumes, volume)
	}

	machine, err := s.machineStore.Create(ctx, cfg.Machine)
	if err != nil {
		//	Todo
		return nil, err
	}
	aggregated.Machine = machine

	return aggregated, nil
}

func (s *Server) CreateMachine(ctx context.Context, req *ori.CreateMachineRequest) (res *ori.CreateMachineResponse, retErr error) {
	log := s.loggerFrom(ctx)

	log.V(1).Info("Getting libvirt machine config")
	cfg, err := s.getLibvirtMachineConfig(ctx, log, req.Machine)
	if err != nil {
		return nil, fmt.Errorf("unable to get libvirt machine config: %w", err)
	}

	log.V(1).Info("Creating libvirt machine")
	aggregateMachine, err := s.createLibvirtMachine(ctx, log, cfg)
	if err != nil {
		return nil, fmt.Errorf("error creating onmetal machine: %w", err)
	}

	oriMachine, err := s.convertMachineToOriMachine(ctx, log, aggregateMachine)
	if err != nil {
		return nil, fmt.Errorf("unable to convert machine: %w", err)
	}

	return &ori.CreateMachineResponse{
		Machine: oriMachine,
	}, nil
}
