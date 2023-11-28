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

	ori "github.com/ironcore-dev/ironcore/iri/apis/machine/v1alpha1"
	"github.com/ironcore-dev/libvirt-provider/pkg/api"
)

func (s *Server) DetachVolume(ctx context.Context, req *ori.DetachVolumeRequest) (*ori.DetachVolumeResponse, error) {
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

	return &ori.DetachVolumeResponse{}, nil
}
