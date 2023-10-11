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
	"fmt"
	"github.com/onmetal/libvirt-driver/pkg/api"
	"github.com/onmetal/libvirt-driver/pkg/store"
	ori "github.com/onmetal/onmetal-api/ori/apis/machine/v1alpha1"
)

var _ ori.MachineRuntimeServer = (*Server)(nil)

type Server struct {
	machineStore store.Store[*api.Machine]
}

type Options struct {
}

func New(machineStore store.Store[*api.Machine], opts Options) (*Server, error) {
	if machineStore == nil {
		return nil, fmt.Errorf("must specify machine store")
	}

	return &Server{
		machineStore: machineStore,
	}, nil
}
