// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package ini_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	machinepoolletv1alpha1 "github.com/ironcore-dev/ironcore/poollet/machinepoollet/api/v1alpha1"
	"github.com/ironcore-dev/libvirt-provider/api"
	providerhost "github.com/ironcore-dev/libvirt-provider/internal/host"
	providernetworkinterface "github.com/ironcore-dev/libvirt-provider/internal/plugins/networkinterface"
	"github.com/ironcore-dev/libvirt-provider/internal/plugins/networkinterface/ini"
	apiutils "github.com/ironcore-dev/provider-utils/apiutils/api"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	machineID = "a3f9b2c1-4d5e-6f70-8192-a3b4c5d6e7f8"
	nicName   = "nic0"
)

var attachmentName = ini.AttachmentName(machineID, nicName)

var _ = Describe("Plugin", func() {
	var host providerhost.LibvirtHost

	BeforeEach(func() {
		var err error
		host, err = providerhost.NewLibvirtAt(GinkgoT().TempDir(), nil)
		Expect(err).NotTo(HaveOccurred())
	})

	Describe("NewPlugin", func() {
		It("rejects an empty plugin path", func() {
			_, err := ini.NewPlugin("")

			Expect(err).To(HaveOccurred())
		})
	})

	Describe("AttachmentName", func() {
		It("is stable", func() {
			Expect(ini.AttachmentName(machineID, nicName)).To(Equal(attachmentName))
		})

		It("differs across network interfaces", func() {
			Expect(ini.AttachmentName(machineID, "nic1")).NotTo(Equal(attachmentName))
		})

		It("differs across machines", func() {
			Expect(ini.AttachmentName("b4f9b2c1-4d5e-6f70-8192-a3b4c5d6e7f8", nicName)).NotTo(Equal(attachmentName))
		})
	})

	Describe("Apply", func() {
		It("returns the tap device as ethernet interface", func(ctx SpecContext) {
			pluginPath, captureDir := writeFakePlugin(`printf '{"tap": {"name": "inisix-tap0"}, "prefixes": ["2001:db8:4000:100::/64"], "ips": ["2001:db8:4000:100::1"]}\n'`)
			plugin := newPlugin(ctx, pluginPath, host)

			nic, err := plugin.Apply(ctx, &api.NetworkInterfaceSpec{Name: nicName}, testMachine())

			Expect(err).NotTo(HaveOccurred())
			Expect(nic).To(Equal(&providernetworkinterface.NetworkInterface{
				Handle:   attachmentName,
				Prefixes: []string{"2001:db8:4000:100::/64"},
				Ips:      []string{"2001:db8:4000:100::1"},
				Ethernet: &providernetworkinterface.Ethernet{Dev: "inisix-tap0"},
			}))

			By("calling ADD with the attachment name")
			Expect(readCapture(captureDir, "command")).To(Equal("ADD"))
			Expect(readCapture(captureDir, "stdin")).To(Equal(fmt.Sprintf(`{"name":%q}`, attachmentName)))

			By("creating the machine network interface dir")
			Expect(host.MachineNetworkInterfaceDir(machineID, nicName)).To(BeADirectory())
		})

		It("accepts a tap device without prefixes", func(ctx SpecContext) {
			pluginPath, _ := writeFakePlugin(`printf '{"tap": {"name": "inisix-tap0"}}\n'`)
			plugin := newPlugin(ctx, pluginPath, host)

			nic, err := plugin.Apply(ctx, &api.NetworkInterfaceSpec{Name: nicName}, testMachine())

			Expect(err).NotTo(HaveOccurred())
			Expect(nic.Prefixes).To(BeEmpty())
			Expect(nic.Ethernet).To(Equal(&providernetworkinterface.Ethernet{Dev: "inisix-tap0"}))
		})

		DescribeTable("sends the hostname",
			func(ctx SpecContext, machine *api.Machine, expectHostname string) {
				pluginPath, captureDir := writeFakePlugin(`printf '{"tap": {"name": "inisix-tap0"}, "prefixes": ["2001:db8:4000:100::/64"]}\n'`)
				plugin := newPlugin(ctx, pluginPath, host)

				_, err := plugin.Apply(ctx, &api.NetworkInterfaceSpec{Name: nicName}, machine)
				Expect(err).NotTo(HaveOccurred())

				expectStdin := fmt.Sprintf(`{"name":%q}`, attachmentName)
				if expectHostname != "" {
					expectStdin = fmt.Sprintf(`{"name":%q,"hostname":%q}`, attachmentName, expectHostname)
				}
				Expect(readCapture(captureDir, "stdin")).To(Equal(expectStdin))
			},
			Entry("explicit guest hostname",
				machineWithHostname("explicit-host", ""), "explicit-host"),
			Entry("explicit guest hostname wins over machine name",
				machineWithHostname("explicit-host", "object-name"), "explicit-host"),
			Entry("machine name from label",
				machineWithHostname("", "object-name"), "object-name"),
			Entry("machine name is sanitized to a DNS label",
				machineWithHostname("", "My.Machine_Example"), "my-machine-example"),
			Entry("machine name is truncated to 63 characters",
				machineWithHostname("", strings.Repeat("a", 80)), strings.Repeat("a", 63)),
			Entry("truncation at an invalid character leaves no trailing dash",
				machineWithHostname("", strings.Repeat("a", 62)+".bb"), strings.Repeat("a", 62)),
			Entry("no hostname derivable",
				machineWithHostname("", ""), ""),
		)

		DescribeTable("fails on plugin errors and invalid results",
			func(ctx SpecContext, body string, expectErr string) {
				pluginPath, _ := writeFakePlugin(body)
				plugin := newPlugin(ctx, pluginPath, host)

				_, err := plugin.Apply(ctx, &api.NetworkInterfaceSpec{Name: nicName}, testMachine())

				Expect(err).To(MatchError(ContainSubstring(expectErr)))
			},
			Entry("json error on stdout",
				`printf '{"error": "no free prefix available"}\n'; exit 1`,
				"no free prefix available"),
			Entry("human readable error on stderr",
				`echo "tuntap: device busy" >&2; exit 1`,
				"tuntap: device busy"),
			Entry("unparsable result",
				`printf 'not-json\n'; exit 0`,
				"error decoding"),
			Entry("invalid prefix",
				`printf '{"tap": {"name": "inisix-tap0"}, "prefixes": ["not-a-prefix"]}\n'; exit 0`,
				"invalid prefix"),
			Entry("invalid ip",
				`printf '{"tap": {"name": "inisix-tap0"}, "prefixes": ["2001:db8:4000:100::/64"], "ips": ["2001:db8:4000:100::1/64"]}\n'; exit 0`,
				"invalid ip"),
			Entry("no device member",
				`printf '{"prefixes": ["2001:db8:4000:100::/64"]}\n'; exit 0`,
				"no device member"),
			Entry("unknown device member",
				`printf '{"foobar": {"name": "inisix-tap0"}, "prefixes": ["2001:db8:4000:100::/64"]}\n'; exit 0`,
				"no device member"),
			Entry("tap device without name",
				`printf '{"tap": {"name": ""}, "prefixes": ["2001:db8:4000:100::/64"]}\n'; exit 0`,
				"tap device without a name"),
		)
	})

	Describe("Delete", func() {
		var plugin providernetworkinterface.Plugin
		var captureDir string

		BeforeEach(func(ctx SpecContext) {
			var pluginPath string
			pluginPath, captureDir = writeFakePlugin(`exit 0`)
			plugin = newPlugin(ctx, pluginPath, host)

			Expect(os.MkdirAll(host.MachineNetworkInterfaceDir(machineID, nicName), 0o777)).To(Succeed())
		})

		It("calls DEL with the attachment name", func(ctx SpecContext) {
			Expect(plugin.Delete(ctx, nicName, machineID)).To(Succeed())

			Expect(readCapture(captureDir, "command")).To(Equal("DEL"))
			Expect(readCapture(captureDir, "stdin")).To(Equal(fmt.Sprintf(`{"name":%q}`, attachmentName)))
		})

		It("removes the machine network interface dir", func(ctx SpecContext) {
			Expect(plugin.Delete(ctx, nicName, machineID)).To(Succeed())

			Expect(host.MachineNetworkInterfaceDir(machineID, nicName)).NotTo(BeAnExistingFile())
		})

		It("is idempotent for unknown attachments", func(ctx SpecContext) {
			Expect(plugin.Delete(ctx, "nonexistent", machineID)).To(Succeed())
		})
	})
})

func newPlugin(ctx SpecContext, pluginPath string, host providerhost.LibvirtHost) providernetworkinterface.Plugin {
	GinkgoHelper()

	plugin, err := ini.NewPlugin(pluginPath)
	Expect(err).NotTo(HaveOccurred())
	Expect(plugin.Init(ctx, host)).To(Succeed())
	return plugin
}

func testMachine() *api.Machine {
	return &api.Machine{Metadata: apiutils.Metadata{ID: machineID}}
}

func machineWithHostname(hostname, machineName string) *api.Machine {
	machine := testMachine()
	if hostname != "" {
		machine.Spec.GuestConfig = &api.MachineGuestConfig{HostName: hostname}
	}
	if machineName != "" {
		machine.Labels = map[string]string{machinepoolletv1alpha1.MachineNameLabel: machineName}
	}
	return machine
}

// writeFakePlugin writes a fake INI plugin executable. The script captures
// INI_COMMAND and stdin into the returned capture directory and executes body
// afterwards.
func writeFakePlugin(body string) (pluginPath, captureDir string) {
	GinkgoHelper()

	captureDir = GinkgoT().TempDir()
	script := fmt.Sprintf(`#!/bin/sh
echo "$INI_COMMAND" >%q
cat >%q
%s
`, filepath.Join(captureDir, "command"), filepath.Join(captureDir, "stdin"), body)

	pluginPath = filepath.Join(captureDir, "inisix")
	Expect(os.WriteFile(pluginPath, []byte(script), 0o755)).To(Succeed())
	return pluginPath, captureDir
}

func readCapture(captureDir, name string) string {
	GinkgoHelper()

	data, err := os.ReadFile(filepath.Join(captureDir, name))
	Expect(err).NotTo(HaveOccurred(), "reading captured %s", name)
	return strings.TrimSpace(string(data))
}
