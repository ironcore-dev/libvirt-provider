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

	"github.com/go-logr/logr"
	machinev1alpha1 "github.com/onmetal/libvirt-driver/driver/api/v1alpha1"
	"github.com/onmetal/libvirt-driver/driver/apiutils"
	"github.com/onmetal/libvirt-driver/pkg/api"
	"github.com/onmetal/libvirt-driver/pkg/store"
	ori "github.com/onmetal/onmetal-api/ori/apis/machine/v1alpha1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/apimachinery/pkg/labels"
)

func (s *Server) getLibvirtMachine(ctx context.Context, id string) (*api.Machine, error) {
	machine, err := s.machineStore.Get(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, status.Errorf(codes.NotFound, "machine %s not found", id)
		}
		return nil, fmt.Errorf("failed to get machine: %w", err)
	}

	if !apiutils.IsManagedBy(machine, machinev1alpha1.MachineManager) {
		return nil, status.Errorf(codes.NotFound, "machine %s not found", id)
	}

	return machine, nil
}

func (s *Server) listMachines(ctx context.Context, log logr.Logger) ([]*ori.Machine, error) {
	machines, err := s.machineStore.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("error listing machines: %w", err)
	}

	var res []*ori.Machine
	for _, machine := range machines {
		if !apiutils.IsManagedBy(machine, machinev1alpha1.MachineManager) {
			continue
		}

		oriMachine, err := s.convertMachineToOriMachine(ctx, log, machine)
		if err != nil {
			return nil, err
		}

		res = append(res, oriMachine)
	}
	return res, nil
}

func (s *Server) filterMachines(machines []*ori.Machine, filter *ori.MachineFilter) []*ori.Machine {
	if filter == nil {
		return machines
	}

	var (
		res []*ori.Machine
		sel = labels.SelectorFromSet(filter.LabelSelector)
	)
	for _, oriMachine := range machines {
		if !sel.Matches(labels.Set(oriMachine.Metadata.Labels)) {
			continue
		}

		res = append(res, oriMachine)
	}
	return res
}

func (s *Server) getMachine(ctx context.Context, log logr.Logger, id string) (*ori.Machine, error) {
	libvirtMachine, err := s.getLibvirtMachine(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get machine: %w", err)
	}

	return s.convertMachineToOriMachine(ctx, log, libvirtMachine)
}

func (s *Server) ListMachines(ctx context.Context, req *ori.ListMachinesRequest) (*ori.ListMachinesResponse, error) {
	log := s.loggerFrom(ctx)

	if filter := req.Filter; filter != nil && filter.Id != "" {
		machine, err := s.getMachine(ctx, log, filter.Id)
		if err != nil {
			if status.Code(err) != codes.NotFound {
				return nil, err
			}
			return &ori.ListMachinesResponse{
				Machines: []*ori.Machine{},
			}, nil
		}

		return &ori.ListMachinesResponse{
			Machines: []*ori.Machine{machine},
		}, nil
	}

	machines, err := s.listMachines(ctx, log)
	if err != nil {
		return nil, err
	}

	machines = s.filterMachines(machines, req.Filter)

	return &ori.ListMachinesResponse{
		Machines: machines,
	}, nil
}
