// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	iri "github.com/ironcore-dev/ironcore/iri/apis/machine/v1alpha1"
	api "github.com/ironcore-dev/libvirt-provider/api"
)

func calcResources(class *iri.MachineClass) (int64, int64) {
	//Todo do some magic
	return class.Capabilities.CpuMillis, class.Capabilities.MemoryBytes
}

func (s *Server) createMachineFromIRIMachine(ctx context.Context, log logr.Logger, iriMachine *iri.Machine) (*api.Machine, error) {
	log.V(2).Info("Getting libvirt machine config")

	switch {
	case iriMachine == nil:
		return nil, fmt.Errorf("iri machine is nil")
	case iriMachine.Spec == nil:
		return nil, fmt.Errorf("iri machine spec is nil")
	case iriMachine.Metadata == nil:
		return nil, fmt.Errorf("iri machine metadata is nil")
	}

	class, found := s.machineClasses.Get(iriMachine.Spec.Class)
	if !found {
		return nil, fmt.Errorf("machine class '%s' not supported", iriMachine.Spec.Class)
	}
	log.V(2).Info("Validated class")

	cpu, memory := calcResources(class)

	power, err := s.getPowerStateFromIRI(iriMachine.Spec.Power)
	if err != nil {
		return nil, fmt.Errorf("failed to get power state: %w", err)
	}

	var volumes []*api.VolumeSpec
	for _, iriVolume := range iriMachine.Spec.Volumes {
		volumeSpec, err := s.getVolumeFromIRIVolume(iriVolume)
		if err != nil {
			return nil, fmt.Errorf("error converting volume: %w", err)
		}

		volumes = append(volumes, volumeSpec)
	}

	var networkInterfaces []*api.NetworkInterfaceSpec
	for _, iriNetworkInterface := range iriMachine.Spec.NetworkInterfaces {
		networkInterfaceSpec := &api.NetworkInterfaceSpec{
			Name:       iriNetworkInterface.Name,
			NetworkId:  iriNetworkInterface.NetworkId,
			Ips:        iriNetworkInterface.Ips,
			Attributes: iriNetworkInterface.Attributes,
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
			Ignition:          iriMachine.Spec.IgnitionData,
			NetworkInterfaces: networkInterfaces,
			GuestAgent:        s.guestAgent,
		},
	}

	if err := api.SetObjectMetadata(machine, iriMachine.Metadata); err != nil {
		return nil, fmt.Errorf("failed to set metadata: %w", err)
	}
	api.SetClassLabel(machine, iriMachine.Spec.Class)
	api.SetManagerLabel(machine, api.MachineManager)

	if iriMachine.Spec.Image != nil {
		machine.Spec.Image = &iriMachine.Spec.Image.Image
	}

	apiMachine, err := s.machineStore.Create(ctx, machine)
	if err != nil {
		return nil, fmt.Errorf("failed to create machine: %w", err)
	}

	return apiMachine, nil
}

func (s *Server) CreateMachine(ctx context.Context, req *iri.CreateMachineRequest) (res *iri.CreateMachineResponse, retErr error) {
	log := s.loggerFrom(ctx)

	log.V(1).Info("Creating machine from iri machine")
	machine, err := s.createMachineFromIRIMachine(ctx, log, req.Machine)
	if err != nil {
		return nil, fmt.Errorf("unable to get libvirt machine config: %w", err)
	}

	log.V(1).Info("Converting machine to iri machine")
	iriMachine, err := s.convertMachineToIRIMachine(ctx, log, machine)
	if err != nil {
		return nil, fmt.Errorf("unable to convert machine: %w", err)
	}

	return &iri.CreateMachineResponse{
		Machine: iriMachine,
	}, nil
}
