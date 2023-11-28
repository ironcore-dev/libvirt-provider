// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

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
