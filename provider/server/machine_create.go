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
	ori "github.com/ironcore-dev/ironcore/iri/apis/machine/v1alpha1"
	"github.com/ironcore-dev/libvirt-provider/pkg/api"
	machinev1alpha1 "github.com/ironcore-dev/libvirt-provider/provider/api/v1alpha1"
	"github.com/ironcore-dev/libvirt-provider/provider/apiutils"
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
		volumeSpec, err := s.getVolumeFromORIVolume(oriVolume)
		if err != nil {
			return nil, fmt.Errorf("error converting volume: %w", err)
		}

		volumes = append(volumes, volumeSpec)
	}

	var networkInterfaces []*api.NetworkInterfaceSpec
	for _, oriNetworkInterface := range oriMachine.Spec.NetworkInterfaces {
		networkInterfaceSpec := &api.NetworkInterfaceSpec{
			Name:       oriNetworkInterface.Name,
			NetworkId:  oriNetworkInterface.NetworkId,
			Ips:        oriNetworkInterface.Ips,
			Attributes: oriNetworkInterface.Attributes,
		}
		networkInterfaces = append(networkInterfaces, networkInterfaceSpec)
	}

	machine := &api.Machine{
		Metadata: api.Metadata{
			ID: s.idGen.Generate(),
		},
		Spec: api.MachineSpec{
			Power:             power,
			CpuMillis:         cpu,
			MemoryBytes:       memory,
			Volumes:           volumes,
			Ignition:          oriMachine.Spec.IgnitionData,
			NetworkInterfaces: networkInterfaces,
		},
	}

	if err := apiutils.SetObjectMetadata(machine, oriMachine.Metadata); err != nil {
		return nil, fmt.Errorf("failed to set metadata: %w", err)
	}
	apiutils.SetClassLabel(machine, oriMachine.Spec.Class)
	apiutils.SetManagerLabel(machine, machinev1alpha1.MachineManager)

	if oriMachine.Spec.Image != nil {
		machine.Spec.Image = &oriMachine.Spec.Image.Image
	}

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
