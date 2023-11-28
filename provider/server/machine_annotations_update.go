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
	"errors"
	"fmt"

	ori "github.com/ironcore-dev/ironcore/iri/apis/machine/v1alpha1"
	"github.com/ironcore-dev/libvirt-provider/pkg/api"
	"github.com/ironcore-dev/libvirt-provider/pkg/store"
	"github.com/ironcore-dev/libvirt-provider/provider/apiutils"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Server) updateAnnotations(ctx context.Context, machine *api.Machine, annotations map[string]string) error {
	if err := apiutils.SetAnnotationsAnnotation(machine, annotations); err != nil {
		return fmt.Errorf("failed to set machine annotations: %w", err)
	}

	if _, err := s.machineStore.Update(ctx, machine); err != nil {
		return fmt.Errorf("failed to update machine: %w", err)
	}

	return nil
}

func (s *Server) UpdateMachineAnnotations(ctx context.Context, req *ori.UpdateMachineAnnotationsRequest) (*ori.UpdateMachineAnnotationsResponse, error) {
	log := s.loggerFrom(ctx)

	log.V(1).Info("Getting machine")
	machine, err := s.machineStore.Get(ctx, req.MachineId)
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			return nil, fmt.Errorf("error getting machine: %w", err)
		}
		return nil, status.Errorf(codes.NotFound, "machine %s not found", req.MachineId)
	}

	if err := s.updateAnnotations(ctx, machine, req.Annotations); err != nil {
		return nil, fmt.Errorf("failed to update machine annotations: %w", err)
	}

	return &ori.UpdateMachineAnnotationsResponse{}, nil
}
