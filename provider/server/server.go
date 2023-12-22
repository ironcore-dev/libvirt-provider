// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"fmt"
	"net/url"
	"path"

	"github.com/digitalocean/go-libvirt"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/ironcore-dev/ironcore/broker/common/idgen"
	"github.com/ironcore-dev/ironcore/broker/common/request"
	iri "github.com/ironcore-dev/ironcore/iri/apis/machine/v1alpha1"
	"github.com/ironcore-dev/libvirt-provider/pkg/api"
	"github.com/ironcore-dev/libvirt-provider/pkg/plugins/volume"
	"github.com/ironcore-dev/libvirt-provider/pkg/store"
	"github.com/ironcore-dev/libvirt-provider/pkg/utils"
	ctrl "sigs.k8s.io/controller-runtime"
)

var _ iri.MachineRuntimeServer = (*Server)(nil)

type Server struct {
	baseURL *url.URL

	idGen idgen.IDGen

	machineStore store.Store[*api.Machine]

	volumePlugins  *volume.PluginManager
	machineClasses MachineClassRegistry

	execRequestCache request.Cache[*iri.ExecRequest]
	libvirt          *libvirt.Libvirt
	virshExecutable  string
}

type Options struct {
	// BaseURL is the base URL in form http(s)://host:port/path?query to produce request URLs relative to.
	BaseURL string

	Libvirt         *libvirt.Libvirt
	VirshExecutable string

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

	baseURL, err := url.ParseRequestURI(opts.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base url %q: %w", opts.BaseURL, err)
	}

	return &Server{
		baseURL:          baseURL,
		idGen:            opts.IDGen,
		libvirt:          opts.Libvirt,
		virshExecutable:  opts.VirshExecutable,
		machineStore:     opts.MachineStore,
		volumePlugins:    opts.VolumePlugins,
		machineClasses:   opts.MachineClasses,
		execRequestCache: request.NewCache[*iri.ExecRequest](),
	}, nil
}

func (s *Server) loggerFrom(ctx context.Context, keysWithValues ...interface{}) logr.Logger {
	return ctrl.LoggerFrom(ctx, keysWithValues...)
}

type MachineClassRegistry interface {
	Get(volumeClassName string) (*iri.MachineClass, bool)
	List() []*iri.MachineClass
}

func (s *Server) buildURL(method string, token string) string {
	return s.baseURL.ResolveReference(&url.URL{
		Path: path.Join(method, token),
	}).String()
}
