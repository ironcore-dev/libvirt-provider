// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"errors"
	"fmt"

	iri "github.com/ironcore-dev/ironcore/iri/apis/machine/v1alpha1"
	"github.com/ironcore-dev/provider-utils/storeutils/store"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Server) DeleteMachine(ctx context.Context, req *iri.DeleteMachineRequest) (*iri.DeleteMachineResponse, error) {
	log := s.loggerFrom(ctx)

	log.V(1).Info("Deleting machine")
	if err := s.machineStore.Delete(ctx, req.MachineId); err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			return nil, fmt.Errorf("error deleting machine: %w", err)
		}
		return nil, status.Errorf(codes.NotFound, "machine %s not found", req.MachineId)
	}

	return &iri.DeleteMachineResponse{}, nil
}
