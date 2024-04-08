// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"fmt"

	iri "github.com/ironcore-dev/ironcore/iri/apis/machine/v1alpha1"
	"github.com/ironcore-dev/libvirt-provider/internal/mcr"
)

func (s *Server) Status(ctx context.Context, req *iri.StatusRequest) (*iri.StatusResponse, error) {
	log := s.loggerFrom(ctx)

	host, err := mcr.GetResources(ctx, s.enableHugepages)
	if err != nil {
		return nil, fmt.Errorf("failed to get host resources: %w", err)
	}

	log.V(1).Info("Listing machine classes")
	machineClassList := s.machineClasses.List()

	var machineClassStatus []*iri.MachineClassStatus
	for _, machineClass := range machineClassList {
		machineClassStatus = append(machineClassStatus, &iri.MachineClassStatus{
			MachineClass: machineClass,
			Quantity:     mcr.GetQuantity(machineClass, host),
		})
	}

	log.V(1).Info("Returning machine classes")
	return &iri.StatusResponse{
		MachineClassStatus: machineClassStatus,
	}, nil
}
