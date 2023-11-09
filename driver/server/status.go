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

	"github.com/onmetal/libvirt-driver/pkg/mcr"
	ori "github.com/onmetal/onmetal-api/ori/apis/machine/v1alpha1"
)

func (s *Server) Status(ctx context.Context, req *ori.StatusRequest) (*ori.StatusResponse, error) {
	log := s.loggerFrom(ctx)

	host, err := mcr.GetResources(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get host resources: %w", err)
	}

	log.V(1).Info("Listing machine classes")
	machineClassList := s.machineClasses.List()

	var machineClassStatus []*ori.MachineClassStatus
	for _, machineClass := range machineClassList {
		machineClassStatus = append(machineClassStatus, &ori.MachineClassStatus{
			MachineClass: machineClass,
			Quantity:     mcr.GetQuantity(machineClass, host),
		})
	}

	log.V(1).Info("Returning machine classes")
	return &ori.StatusResponse{
		MachineClassStatus: machineClassStatus,
	}, nil
}
