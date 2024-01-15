// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"fmt"

	iri "github.com/ironcore-dev/ironcore/iri/apis/machine/v1alpha1"
)

func (s *Server) AttachNetworkInterface(ctx context.Context, req *iri.AttachNetworkInterfaceRequest) (res *iri.AttachNetworkInterfaceResponse, retErr error) {
	log := s.loggerFrom(ctx)
	log.V(1).Info("Attaching NIC to machine")

	if req == nil {
		return nil, fmt.Errorf("AttachNetworkInterfaceRequest is nil")
	}

	apiMachine, err := s.machineStore.Get(ctx, req.MachineId)
	if err != nil {
		return nil, fmt.Errorf("failed to get machine: %w", err)
	}

	nicSpec, err := s.getNICFromIRINIC(req.NetworkInterface)
	if err != nil {
		return nil, fmt.Errorf("failed to get nic from iri nic: %w", err)
	}

	apiMachine.Spec.NetworkInterfaces = append(apiMachine.Spec.NetworkInterfaces, nicSpec)

	if _, err := s.machineStore.Update(ctx, apiMachine); err != nil {
		return nil, fmt.Errorf("failed to update machine: %w", err)
	}

	return &iri.AttachNetworkInterfaceResponse{}, nil
}
