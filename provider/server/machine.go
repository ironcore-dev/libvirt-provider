// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	ori "github.com/ironcore-dev/ironcore/iri/apis/machine/v1alpha1"
	"github.com/ironcore-dev/libvirt-provider/pkg/api"
	"github.com/ironcore-dev/libvirt-provider/provider/apiutils"
)

func (s *Server) convertMachineToOriMachine(ctx context.Context, log logr.Logger, machine *api.Machine) (*ori.Machine, error) {
	metadata, err := apiutils.GetObjectMetadata(machine.Metadata)
	if err != nil {
		return nil, fmt.Errorf("error getting ori metadata: %w", err)
	}

	spec, err := s.getOriMachineSpec(machine)
	if err != nil {
		return nil, fmt.Errorf("error getting ori resources: %w", err)
	}

	state, err := s.getOriMachineStatus(machine)
	if err != nil {
		return nil, fmt.Errorf("error getting ori state: %w", err)
	}

	return &ori.Machine{
		Metadata: metadata,
		Spec:     spec,
		Status:   state,
	}, nil
}

func (s *Server) getOriMachineSpec(machine *api.Machine) (*ori.MachineSpec, error) {
	class, ok := apiutils.GetClassLabel(machine)
	if !ok {
		return nil, fmt.Errorf("failed to get machine class")
	}

	power, err := s.getOriPower(machine.Spec.Power)
	if err != nil {
		return nil, fmt.Errorf("failed to get power state: %w", err)
	}

	var imageSpec *ori.ImageSpec
	if image := machine.Spec.Image; image != nil {
		imageSpec = &ori.ImageSpec{
			Image: *image,
		}
	}

	spec := &ori.MachineSpec{
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

func (s *Server) getOriMachineStatus(machine *api.Machine) (*ori.MachineStatus, error) {
	state, err := s.getOriState(machine.Status.State)
	if err != nil {
		return nil, fmt.Errorf("failed to machine state: %w", err)
	}

	return &ori.MachineStatus{
		ObservedGeneration: machine.Generation,
		State:              state,
		ImageRef:           machine.Status.ImageRef,
		//Todo
		Volumes: nil,
		//Todo
		NetworkInterfaces: nil,
	}, nil
}

func (s *Server) getOriState(state api.MachineState) (ori.MachineState, error) {
	switch state {
	case api.MachineStatePending:
		return ori.MachineState_MACHINE_PENDING, nil
	case api.MachineStateRunning:
		return ori.MachineState_MACHINE_RUNNING, nil
	case api.MachineStateSuspended:
		return ori.MachineState_MACHINE_SUSPENDED, nil
	case api.MachineStateTerminated:
		return ori.MachineState_MACHINE_TERMINATED, nil
	default:
		return 0, fmt.Errorf("unknown machine state '%q'", state)
	}
}

func (s *Server) getOriPower(state api.PowerState) (ori.Power, error) {
	switch state {
	case api.PowerStatePowerOn:
		return ori.Power_POWER_ON, nil
	case api.PowerStatePowerOff:
		return ori.Power_POWER_OFF, nil
	default:
		return 0, fmt.Errorf("unknown machine power state '%q'", state)
	}
}

func (s *Server) getPowerStateFromOri(power ori.Power) (api.PowerState, error) {
	switch power {
	case ori.Power_POWER_ON:
		return api.PowerStatePowerOn, nil
	case ori.Power_POWER_OFF:
		return api.PowerStatePowerOff, nil
	default:
		return 0, fmt.Errorf("unknown ori power state '%q'", power)
	}
}

func (s *Server) getVolumeFromORIVolume(oriVolume *ori.Volume) (*api.VolumeSpec, error) {
	if oriVolume == nil {
		return nil, fmt.Errorf("original volume is nil")
	}

	var emptyDiskSpec *api.EmptyDiskSpec
	if emptyDisk := oriVolume.EmptyDisk; emptyDisk != nil {
		emptyDiskSpec = &api.EmptyDiskSpec{
			Size: emptyDisk.SizeBytes,
		}
	}

	var connectionSpec *api.VolumeConnection
	if connection := oriVolume.Connection; connection != nil {
		connectionSpec = &api.VolumeConnection{
			Driver:     connection.Driver,
			Handle:     connection.Handle,
			Attributes: connection.Attributes,
			SecretData: connection.SecretData,
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

	return volumeSpec, nil
}
