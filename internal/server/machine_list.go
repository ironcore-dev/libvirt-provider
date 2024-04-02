// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	iri "github.com/ironcore-dev/ironcore/iri/apis/machine/v1alpha1"
	"github.com/ironcore-dev/libvirt-provider/api"
	"github.com/ironcore-dev/libvirt-provider/internal/store"
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

	if !api.IsManagedBy(machine, api.MachineManager) {
		return nil, status.Errorf(codes.NotFound, "machine %s not found", id)
	}

	return machine, nil
}

func (s *Server) listMachines(ctx context.Context, log logr.Logger) ([]*iri.Machine, error) {
	machines, err := s.machineStore.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("error listing machines: %w", err)
	}

	var res []*iri.Machine
	for _, machine := range machines {
		if !api.IsManagedBy(machine, api.MachineManager) {
			continue
		}

		iriMachine, err := s.convertMachineToIRIMachine(ctx, log, machine)
		if err != nil {
			return nil, err
		}

		res = append(res, iriMachine)
	}
	return res, nil
}

func (s *Server) filterMachines(machines []*iri.Machine, filter *iri.MachineFilter) []*iri.Machine {
	if filter == nil {
		return machines
	}

	var (
		res []*iri.Machine
		sel = labels.SelectorFromSet(filter.LabelSelector)
	)
	for _, iriMachine := range machines {
		if !sel.Matches(labels.Set(iriMachine.Metadata.Labels)) {
			continue
		}

		res = append(res, iriMachine)
	}
	return res
}

func (s *Server) getMachine(ctx context.Context, log logr.Logger, id string) (*iri.Machine, error) {
	libvirtMachine, err := s.getLibvirtMachine(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get machine: %w", err)
	}

	return s.convertMachineToIRIMachine(ctx, log, libvirtMachine)
}

func (s *Server) ListMachines(ctx context.Context, req *iri.ListMachinesRequest) (*iri.ListMachinesResponse, error) {
	log := s.loggerFrom(ctx)

	if filter := req.Filter; filter != nil && filter.Id != "" {
		machine, err := s.getMachine(ctx, log, filter.Id)
		if err != nil {
			if status.Code(err) != codes.NotFound {
				return nil, err
			}
			return &iri.ListMachinesResponse{
				Machines: []*iri.Machine{},
			}, nil
		}

		return &iri.ListMachinesResponse{
			Machines: []*iri.Machine{machine},
		}, nil
	}

	machines, err := s.listMachines(ctx, log)
	if err != nil {
		return nil, err
	}

	machines = s.filterMachines(machines, req.Filter)

	return &iri.ListMachinesResponse{
		Machines: machines,
	}, nil
}
