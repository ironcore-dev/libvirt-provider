// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/ironcore-dev/ironcore/broker/common/idgen"
	ori "github.com/ironcore-dev/ironcore/iri/apis/machine/v1alpha1"
	"github.com/ironcore-dev/libvirt-provider/pkg/api"
	"github.com/ironcore-dev/libvirt-provider/pkg/plugins/volume"
	"github.com/ironcore-dev/libvirt-provider/pkg/store"
	"github.com/ironcore-dev/libvirt-provider/pkg/utils"
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
