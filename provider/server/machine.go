// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	iri "github.com/ironcore-dev/ironcore/iri/apis/machine/v1alpha1"
	"github.com/ironcore-dev/libvirt-provider/pkg/api"
	"github.com/ironcore-dev/libvirt-provider/provider/apiutils"
)

func (s *Server) convertMachineToIRIMachine(ctx context.Context, log logr.Logger, machine *api.Machine) (*iri.Machine, error) {
	metadata, err := apiutils.GetObjectMetadata(machine.Metadata)
	if err != nil {
		return nil, fmt.Errorf("error getting iri metadata: %w", err)
	}

	spec, err := s.getIRIMachineSpec(machine)
	if err != nil {
		return nil, fmt.Errorf("error getting iri resources: %w", err)
	}

	state, err := s.getIRIMachineStatus(machine)
	if err != nil {
		return nil, fmt.Errorf("error getting iri state: %w", err)
	}

	return &iri.Machine{
		Metadata: metadata,
		Spec:     spec,
		Status:   state,
	}, nil
}

func (s *Server) getIRIMachineSpec(machine *api.Machine) (*iri.MachineSpec, error) {
	class, ok := apiutils.GetClassLabel(machine)
	if !ok {
		return nil, fmt.Errorf("failed to get machine class")
	}

	power, err := s.getIRIPower(machine.Spec.Power)
	if err != nil {
		return nil, fmt.Errorf("failed to get power state: %w", err)
	}

	var imageSpec *iri.ImageSpec
	if image := machine.Spec.Image; image != nil {
		imageSpec = &iri.ImageSpec{
			Image: *image,
		}
	}

	spec := &iri.MachineSpec{
		Power:        power,
		Image:        imageSpec,
		Class:        class,
		IgnitionData: machine.Spec.Ignition,
		//ToDo
		Volumes:           nil,
		NetworkInterfaces: nil,
	}

	return spec, nil
}

func (s *Server) getIRIMachineStatus(machine *api.Machine) (*iri.MachineStatus, error) {
	state, err := s.getIRIState(machine.Status.State)
	if err != nil {
		return nil, fmt.Errorf("failed to machine state: %w", err)
	}

	return &iri.MachineStatus{
		ObservedGeneration: machine.Generation,
		State:              state,
		ImageRef:           machine.Status.ImageRef,
		//Todo
		Volumes: nil,
		//Todo
		NetworkInterfaces: nil,
	}, nil
}

func (s *Server) getIRIState(state api.MachineState) (iri.MachineState, error) {
	switch state {
	case api.MachineStatePending:
		return iri.MachineState_MACHINE_PENDING, nil
	case api.MachineStateRunning:
		return iri.MachineState_MACHINE_RUNNING, nil
	case api.MachineStateSuspended:
		return iri.MachineState_MACHINE_SUSPENDED, nil
	case api.MachineStateTerminated:
		return iri.MachineState_MACHINE_TERMINATED, nil
	default:
		return 0, fmt.Errorf("unknown machine state '%q'", state)
	}
}

func (s *Server) getIRIPower(state api.PowerState) (iri.Power, error) {
	switch state {
	case api.PowerStatePowerOn:
		return iri.Power_POWER_ON, nil
	case api.PowerStatePowerOff:
		return iri.Power_POWER_OFF, nil
	default:
		return 0, fmt.Errorf("unknown machine power state '%q'", state)
	}
}

func (s *Server) getPowerStateFromIRI(power iri.Power) (api.PowerState, error) {
	switch power {
	case iri.Power_POWER_ON:
		return api.PowerStatePowerOn, nil
	case iri.Power_POWER_OFF:
		return api.PowerStatePowerOff, nil
	default:
		return 0, fmt.Errorf("unknown iri power state '%q'", power)
	}
}

func (s *Server) getVolumeFromIRIVolume(iriVolume *iri.Volume) (*api.VolumeSpec, error) {
	if iriVolume == nil {
		return nil, fmt.Errorf("original volume is nil")
	}

	var emptyDiskSpec *api.EmptyDiskSpec
	if emptyDisk := iriVolume.EmptyDisk; emptyDisk != nil {
		emptyDiskSpec = &api.EmptyDiskSpec{
			Size: emptyDisk.SizeBytes,
		}
	}

	var connectionSpec *api.VolumeConnection
	if connection := iriVolume.Connection; connection != nil {
		connectionSpec = &api.VolumeConnection{
			Driver:         connection.Driver,
			Handle:         connection.Handle,
			Attributes:     connection.Attributes,
			SecretData:     connection.SecretData,
			EncryptionData: connection.EncryptionData,
		}
	}

	volumeSpec := &api.VolumeSpec{
		Name:       iriVolume.Name,
		Device:     iriVolume.Device,
		EmptyDisk:  emptyDiskSpec,
		Connection: connectionSpec,
	}

	if _, err := s.volumePlugins.FindPluginBySpec(volumeSpec); err != nil {
		return nil, fmt.Errorf("failed to find volume plugin: %w", err)
	}

	return volumeSpec, nil
}
