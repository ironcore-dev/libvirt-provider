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

	ori "github.com/onmetal/onmetal-api/ori/apis/machine/v1alpha1"
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
