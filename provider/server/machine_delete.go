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

	ori "github.com/ironcore-dev/ironcore/iri/apis/machine/v1alpha1"
	"github.com/ironcore-dev/libvirt-provider/pkg/store"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Server) DeleteMachine(ctx context.Context, req *ori.DeleteMachineRequest) (*ori.DeleteMachineResponse, error) {
	log := s.loggerFrom(ctx)

	log.V(1).Info("Deleting machine")
	if err := s.machineStore.Delete(ctx, req.MachineId); err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			return nil, fmt.Errorf("error deleting machine: %w", err)
		}
		return nil, status.Errorf(codes.NotFound, "machine %s not found", req.MachineId)
	}

	return &ori.DeleteMachineResponse{}, nil
}
