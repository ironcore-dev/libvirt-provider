// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"fmt"

	ori "github.com/ironcore-dev/ironcore/iri/apis/machine/v1alpha1"
)

func (s *Server) AttachVolume(ctx context.Context, req *ori.AttachVolumeRequest) (*ori.AttachVolumeResponse, error) {
	log := s.loggerFrom(ctx)
	log.V(1).Info("Attaching volume to machine")

	if req == nil || req.MachineId == "" || req.Volume == nil {
		return nil, fmt.Errorf("invalid request")
	}

	apiMachine, err := s.machineStore.Get(ctx, req.MachineId)
	if err != nil {
		return nil, fmt.Errorf("failed to get machine: %w", err)
	}

	volumeSpec, err := s.getVolumeFromORIVolume(req.Volume)
	if err != nil {
		return nil, fmt.Errorf("error converting volume: %w", err)
	}

	apiMachine.Spec.Volumes = append(apiMachine.Spec.Volumes, volumeSpec)

	if _, err := s.machineStore.Update(ctx, apiMachine); err != nil {
		return nil, fmt.Errorf("failed to update machine with new volume: %w", err)
	}

	return &ori.AttachVolumeResponse{}, nil
}
