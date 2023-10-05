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
	"github.com/onmetal/libvirt-driver/pkg/api"
	ori "github.com/onmetal/onmetal-api/ori/apis/machine/v1alpha1"
	"github.com/onmetal/onmetal-api/ori/apis/meta/v1alpha1"
)

func (s *Server) CreateMachine(ctx context.Context, req *ori.CreateMachineRequest) (res *ori.CreateMachineResponse, retErr error) {

	machine, err := s.machineStore.Create(ctx, &api.Machine{
		Metadata: api.Metadata{
			ID:          "test-id-1",
			Annotations: req.Machine.GetMetadata().GetAnnotations(),
			Labels:      req.Machine.GetMetadata().GetLabels(),
		},
		Spec:   api.MachineSpec{},
		Status: api.MachineStatus{},
	})
	if err != nil {
		return nil, err
	}

	return &ori.CreateMachineResponse{
		Machine: &ori.Machine{
			Metadata: &v1alpha1.ObjectMetadata{
				Id:          machine.ID,
				Annotations: machine.Annotations,
				Labels:      machine.Labels,
				Generation:  machine.Generation,
			},
			Spec: &ori.MachineSpec{
				Power:             0,
				Image:             nil,
				Class:             "",
				IgnitionData:      nil,
				Volumes:           nil,
				NetworkInterfaces: nil,
			},
			Status: &ori.MachineStatus{
				ObservedGeneration: machine.Generation,
				State:              0,
			},
		},
	}, nil
}
