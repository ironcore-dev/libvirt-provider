// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"fmt"

	iri "github.com/ironcore-dev/ironcore/iri/apis/machine/v1alpha1"
	"github.com/ironcore-dev/libvirt-provider/api"
	apiutils "github.com/ironcore-dev/provider-utils/apiutils/api"
)

func (s *Server) updateAnnotations(ctx context.Context, machine *api.Machine, annotations map[string]string) error {
	if err := apiutils.SetAnnotationsAnnotation(machine, api.AnnotationsAnnotation, annotations); err != nil {
		return fmt.Errorf("failed to set machine annotations: %w", err)
	}

	if _, err := s.machineStore.Update(ctx, machine); err != nil {
		return fmt.Errorf("failed to update machine: %w", err)
	}

	return nil
}

func (s *Server) UpdateMachineAnnotations(ctx context.Context, req *iri.UpdateMachineAnnotationsRequest) (*iri.UpdateMachineAnnotationsResponse, error) {
	log := s.loggerFrom(ctx)

	log.V(1).Info("Getting machine")
	machine, err := s.machineStore.Get(ctx, req.MachineId)
	if err != nil {
		return nil, convertInternalErrorToGRPC(fmt.Errorf("failed to get machine '%s': %w", req.MachineId, err))
	}

	if err := s.updateAnnotations(ctx, machine, req.Annotations); err != nil {
		return nil, convertInternalErrorToGRPC(fmt.Errorf("failed to update machine annotations: %w", err))
	}

	return &iri.UpdateMachineAnnotationsResponse{}, nil
}
