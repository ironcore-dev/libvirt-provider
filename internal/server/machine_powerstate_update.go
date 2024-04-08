// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"errors"
	"fmt"

	iri "github.com/ironcore-dev/ironcore/iri/apis/machine/v1alpha1"
	"github.com/ironcore-dev/libvirt-provider/api"
	"github.com/ironcore-dev/libvirt-provider/internal/store"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Server) updatePowerState(ctx context.Context, machine *api.Machine, iriPower iri.Power) error {
	power, err := s.getPowerStateFromIRI(iriPower)
	if err != nil {
		return fmt.Errorf("failed to get power state: %w", err)
	}

	machine.Spec.Power = power

	if _, err = s.machineStore.Update(ctx, machine); err != nil {
		return fmt.Errorf("failed to update machine: %w", err)
	}

	return nil
}

func (s *Server) UpdateMachinePower(ctx context.Context, req *iri.UpdateMachinePowerRequest) (*iri.UpdateMachinePowerResponse, error) {
	log := s.loggerFrom(ctx)

	log.V(1).Info("Getting machine")
	machine, err := s.machineStore.Get(ctx, req.MachineId)
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			return nil, fmt.Errorf("error getting machine: %w", err)
		}
		return nil, status.Errorf(codes.NotFound, "machine %s not found", req.MachineId)
	}

	if err := s.updatePowerState(ctx, machine, req.Power); err != nil {
		return nil, fmt.Errorf("failed to update power state: %w", err)
	}

	return &iri.UpdateMachinePowerResponse{}, nil
}
