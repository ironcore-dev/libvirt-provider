// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

// Package isolated implements a stateless network interface plugin that
// connects guests to libvirt user-mode networking.
//
// TODO: remove once inisix lands.
package isolated

import (
	"context"

	"github.com/ironcore-dev/provider-utils/plugin"
)

const (
	pluginName = "isolated"

	// NetdevName is the name of the Netdev reported by the plugin. The provider
	// renders a Netdev with this name as user-mode networking.
	NetdevName = "isolated"
)

type Plugin struct{}

var _ plugin.NetworkInterface = (*Plugin)(nil)

func New() *Plugin {
	return &Plugin{}
}

func (p *Plugin) Name() string {
	return pluginName
}

func (p *Plugin) Apply(_ context.Context, _ *plugin.NetworkInterfaceSpec, _ string) (*plugin.NetworkInterfaceStatus, error) {
	return &plugin.NetworkInterfaceStatus{
		State:  plugin.NetworkInterfaceStateReady,
		Netdev: &plugin.Netdev{Name: NetdevName},
	}, nil
}

func (p *Plugin) Delete(_ context.Context, _ *plugin.NetworkInterfaceSpec, _ string) error {
	return nil
}
