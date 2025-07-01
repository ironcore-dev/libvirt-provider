// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	iri "github.com/ironcore-dev/ironcore/iri/apis/machine/v1alpha1"
	"github.com/ironcore-dev/libvirt-provider/api"
)

func (s *Server) convertMachineToIRIMachine(ctx context.Context, log logr.Logger, machine *api.Machine) (*iri.Machine, error) {
	metadata, err := api.GetObjectMetadata(machine.Metadata)
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
	class, ok := api.GetClassLabel(machine)
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
		Power:             power,
		Image:             imageSpec,
		Class:             class,
		IgnitionData:      machine.Spec.Ignition,
		Volumes:           s.getIRIVolumeSpec(machine),
		NetworkInterfaces: s.getIRINICSpec(machine),
	}

	return spec, nil
}

func (s *Server) getIRIVolumeSpec(machine *api.Machine) []*iri.Volume {
	var volumes []*iri.Volume
	for _, volume := range machine.Spec.Volumes {
		var emptyDisk *iri.EmptyDisk
		if volume.EmptyDisk != nil {
			emptyDisk = &iri.EmptyDisk{
				SizeBytes: volume.EmptyDisk.Size,
			}
		}

		var connection *iri.VolumeConnection
		if volumeConnection := volume.Connection; volumeConnection != nil {
			connection = &iri.VolumeConnection{
				Driver:                volumeConnection.Driver,
				Handle:                volumeConnection.Handle,
				Attributes:            volumeConnection.Attributes,
				SecretData:            volumeConnection.SecretData,
				EncryptionData:        volumeConnection.EncryptionData,
				EffectiveStorageBytes: volumeConnection.EffectiveStorageBytes,
			}
		}

		volumes = append(volumes, &iri.Volume{
			Name:       volume.Name,
			Device:     volume.Device,
			EmptyDisk:  emptyDisk,
			Connection: connection,
		})
	}

	return volumes
}

func (s *Server) getIRINICSpec(machine *api.Machine) []*iri.NetworkInterface {
	var nics []*iri.NetworkInterface
	for _, nic := range machine.Spec.NetworkInterfaces {
		nics = append(nics, &iri.NetworkInterface{
			Name:       nic.Name,
			NetworkId:  nic.NetworkId,
			Ips:        nic.Ips,
			Attributes: nic.Attributes,
		})
	}

	return nics
}

func (s *Server) getIRIVolumeStatus(machine *api.Machine) ([]*iri.VolumeStatus, error) {
	var volumes []*iri.VolumeStatus
	for _, volume := range machine.Status.VolumeStatus {
		state, err := s.getIRIVolumeState(volume.State)
		if err != nil {
			return nil, fmt.Errorf("failed to get volume state: %w", err)
		}

		volumes = append(volumes, &iri.VolumeStatus{
			Name:   volume.Name,
			Handle: volume.Handle,
			State:  state,
		})
	}

	return volumes, nil
}

func (s *Server) getIRINICStatus(machine *api.Machine) ([]*iri.NetworkInterfaceStatus, error) {
	var nics []*iri.NetworkInterfaceStatus
	for _, nic := range machine.Status.NetworkInterfaceStatus {
		state, err := s.getIRINICState(nic.State)
		if err != nil {
			return nil, fmt.Errorf("failed to get nic state: %w", err)
		}

		nics = append(nics, &iri.NetworkInterfaceStatus{
			Name:   nic.Name,
			Handle: nic.Handle,
			State:  state,
		})
	}

	return nics, nil
}

func (s *Server) getIRIMachineStatus(machine *api.Machine) (*iri.MachineStatus, error) {
	state, err := s.getIRIState(machine.Status.State)
	if err != nil {
		return nil, fmt.Errorf("failed to get machine state: %w", err)
	}

	volumes, err := s.getIRIVolumeStatus(machine)
	if err != nil {
		return nil, fmt.Errorf("failed to get volume status: %w", err)
	}

	nics, err := s.getIRINICStatus(machine)
	if err != nil {
		return nil, fmt.Errorf("failed to get network interface status: %w", err)
	}

	return &iri.MachineStatus{
		ObservedGeneration: machine.Generation,
		State:              state,
		ImageRef:           machine.Status.ImageRef,
		Volumes:            volumes,
		NetworkInterfaces:  nics,
	}, nil
}

func (s *Server) getIRINICState(state api.NetworkInterfaceState) (iri.NetworkInterfaceState, error) {
	switch state {
	case api.NetworkInterfaceStateAttached:
		return iri.NetworkInterfaceState_NETWORK_INTERFACE_ATTACHED, nil
	case api.NetworkInterfaceStatePending:
		return iri.NetworkInterfaceState_NETWORK_INTERFACE_PENDING, nil
	default:
		return 0, fmt.Errorf("unknown network interface state '%q'", state)
	}
}

func (s *Server) getIRIVolumeState(state api.VolumeState) (iri.VolumeState, error) {
	switch state {
	case api.VolumeStateAttached:
		return iri.VolumeState_VOLUME_ATTACHED, nil
	case api.VolumeStatePending:
		return iri.VolumeState_VOLUME_PENDING, nil
	default:
		return 0, fmt.Errorf("unknown volume state '%q'", state)
	}
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
	case api.MachineStateTerminating:
		return iri.MachineState_MACHINE_TERMINATING, nil
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
			Driver:                connection.Driver,
			Handle:                connection.Handle,
			Attributes:            connection.Attributes,
			SecretData:            connection.SecretData,
			EncryptionData:        connection.EncryptionData,
			EffectiveStorageBytes: connection.EffectiveStorageBytes,
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

func (s *Server) getNICFromIRINIC(iriNIC *iri.NetworkInterface) (*api.NetworkInterfaceSpec, error) {
	if iriNIC == nil {
		return nil, fmt.Errorf("networkInterface is nil")
	}

	return &api.NetworkInterfaceSpec{
		Name:       iriNIC.Name,
		NetworkId:  iriNIC.NetworkId,
		Ips:        iriNIC.Ips,
		Attributes: iriNIC.Attributes,
	}, nil
}
