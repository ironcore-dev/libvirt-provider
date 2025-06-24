// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"fmt"

	iri "github.com/ironcore-dev/ironcore/iri/apis/machine/v1alpha1"
	"github.com/ironcore-dev/libvirt-provider/api"
)

func (s *Server) DetachNetworkInterface(
	ctx context.Context,
	req *iri.DetachNetworkInterfaceRequest,
) (*iri.DetachNetworkInterfaceResponse, error) {
	log := s.loggerFrom(ctx)
	log.V(1).Info("Detaching nic from machine")

	if req == nil {
		return nil, convertInternalErrorToGRPC(fmt.Errorf("DetachNetworkInterface is nil: %w", ErrInvalidRequest))
	}

	apiMachine, err := s.machineStore.Get(ctx, req.MachineId)
	if err != nil {
		return nil, convertInternalErrorToGRPC(fmt.Errorf("failed to get machine '%s': %w", req.MachineId, err))
	}

	var updatedNICS []*api.NetworkInterfaceSpec
	found := false
	for _, nic := range apiMachine.Spec.NetworkInterfaces {
		if nic.Name != req.Name {
			updatedNICS = append(updatedNICS, nic)
		} else {
			found = true
		}
	}

	if !found {
		return nil, convertInternalErrorToGRPC(fmt.Errorf("nic '%s' not found in machine '%s': %w", req.Name, req.MachineId, ErrNicNotFound))
	}

	apiMachine.Spec.NetworkInterfaces = updatedNICS

	if _, err := s.machineStore.Update(ctx, apiMachine); err != nil {
		return nil, convertInternalErrorToGRPC(fmt.Errorf("failed to update machine: %w", err))
	}

	return &iri.DetachNetworkInterfaceResponse{}, nil
}
