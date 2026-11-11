// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package networkinterfaceplugin

import (
	"context"
	"testing"

	"github.com/ironcore-dev/libvirt-provider/internal/plugins/networkinterface/isolated"
	. "github.com/onsi/gomega"
	"github.com/spf13/pflag"
)

func TestOptions(t *testing.T) {
	t.Run("defaults to apinet", func(t *testing.T) {
		g := NewWithT(t)
		opts := NewDefaultOptions()
		opts.AddFlags(pflag.NewFlagSet("test", pflag.ContinueOnError))
		g.Expect(opts.PluginName).To(Equal(PluginNameAPInet))
	})

	t.Run("plugin name flag", func(t *testing.T) {
		g := NewWithT(t)
		opts := NewDefaultOptions()
		fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
		opts.AddFlags(fs)
		g.Expect(fs.Parse([]string{"--network-interface-plugin-name=isolated", "--apinet-node-name=node"})).To(Succeed())
		g.Expect(opts.PluginName).To(Equal(PluginNameIsolated))
		g.Expect(opts.APInetNodeName).To(Equal("node"))
	})

	t.Run("isolated", func(t *testing.T) {
		g := NewWithT(t)
		opts := &Options{PluginName: PluginNameIsolated}
		p, configCtrl, err := opts.New(context.Background(), Deps{})
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(p).To(BeAssignableToTypeOf(&isolated.Plugin{}))
		g.Expect(configCtrl).To(BeNil())
	})

	t.Run("apinet without node name", func(t *testing.T) {
		g := NewWithT(t)
		opts := &Options{PluginName: PluginNameAPInet}
		_, _, err := opts.New(context.Background(), Deps{})
		g.Expect(err).To(MatchError(ContainSubstring("apinet-node-name")))
	})

	t.Run("unknown plugin", func(t *testing.T) {
		g := NewWithT(t)
		opts := &Options{PluginName: "unknown"}
		_, _, err := opts.New(context.Background(), Deps{})
		g.Expect(err).To(HaveOccurred())
	})
}
