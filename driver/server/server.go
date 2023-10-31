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
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/onmetal/libvirt-driver/pkg/api"
	"github.com/onmetal/libvirt-driver/pkg/plugins/volume"
	"github.com/onmetal/libvirt-driver/pkg/store"
	"github.com/onmetal/libvirt-driver/pkg/utils"
	"github.com/onmetal/onmetal-api/broker/common/idgen"
	ori "github.com/onmetal/onmetal-api/ori/apis/machine/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
)

var _ ori.MachineRuntimeServer = (*Server)(nil)

type Server struct {
	idGen idgen.IDGen

	machineStore store.Store[*api.Machine]

	volumePlugins  *volume.PluginManager
	machineClasses MachineClassRegistry
}

type Options struct {
	IDGen idgen.IDGen

	MachineStore store.Store[*api.Machine]

	MachineClasses MachineClassRegistry

	VolumePlugins *volume.PluginManager
}

func setOptionsDefaults(o *Options) {
	if o.IDGen == nil {
		o.IDGen = utils.IdGenerateFunc(uuid.NewString)
	}
}

func New(opts Options) (*Server, error) {

	setOptionsDefaults(&opts)

	return &Server{
		idGen:          opts.IDGen,
		machineStore:   opts.MachineStore,
		volumePlugins:  opts.VolumePlugins,
		machineClasses: opts.MachineClasses,
	}, nil
}

func (s *Server) loggerFrom(ctx context.Context, keysWithValues ...interface{}) logr.Logger {
	return ctrl.LoggerFrom(ctx, keysWithValues...)
}

type MachineClassRegistry interface {
	Get(volumeClassName string) (*ori.MachineClass, bool)
	List() []*ori.MachineClass
}
