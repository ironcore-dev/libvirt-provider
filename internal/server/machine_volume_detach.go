// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"fmt"

	iri "github.com/ironcore-dev/ironcore/iri/apis/machine/v1alpha1"
	"github.com/ironcore-dev/libvirt-provider/api"
)

func (s *Server) DetachVolume(ctx context.Context, req *iri.DetachVolumeRequest) (*iri.DetachVolumeResponse, error) {
	log := s.loggerFrom(ctx)
	log.V(1).Info("Detaching volume from machine")

	if req == nil || req.MachineId == "" || req.Name == "" {
		return nil, fmt.Errorf("invalid request")
	}

	apiMachine, err := s.machineStore.Get(ctx, req.MachineId)
	if err != nil {
		return nil, fmt.Errorf("failed to get machine: %w", err)
	}

	var updatedVolumes []*api.VolumeSpec
	found := false
	for _, volume := range apiMachine.Spec.Volumes {
		if volume.Name != req.Name {
			updatedVolumes = append(updatedVolumes, volume)
		} else {
			found = true
		}
	}

	if !found {
		return nil, fmt.Errorf("volume '%s' not found in machine '%s'", req.Name, req.MachineId)
	}

	apiMachine.Spec.Volumes = updatedVolumes

	if _, err := s.machineStore.Update(ctx, apiMachine); err != nil {
		return nil, fmt.Errorf("failed to update machine after detaching volume: %w", err)
	}

	return &iri.DetachVolumeResponse{}, nil
}
