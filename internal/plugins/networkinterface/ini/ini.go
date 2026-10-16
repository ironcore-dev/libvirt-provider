// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

// Package ini implements a generic shell-out interface for provisioning network
// interfaces. Instead of having to maintain the business logic of the plugin
// in-tree a dedicated binary is provided and configured that exposes the
// expected interface.
//
// Currently, the plugin expects two commands to be available: ADD and DEL. Each
// command accepts a configuration on stdin and may produce a result or error
// response on stdout. If the exist code is non-zero, the plugin is expected to
// produce a error message in the following format:
//
//	{"error": "<message>"}
//
// When adding a new interface the plugin is called with INI_COMMAND=ADD and the
// configuration of the interface on stdin:
//
//	{
//	  "name": "<name>",
//	  "hostname": "<hostname>"
//	}
//
// On success, the plugin returns the following configuration:
//
//	{
//	  "handle": "<handle>",
//	  "tap": {
//	    "name": "<name>"
//	  },
//	  "prefixes": ["<prefix>"],
//	  "ips": ["<ip>"]
//	}
//
// The fd-end of the tap device is to be consumed by the hypervisor for
// connecting it to the guest-side. No other process must use the fd-end of this
// tap device.
//
// A plugin can only return one type of interface and currently only support for
// tap devices is included. Future additions would be:
//
//   - `"pci": {"address": "<bfd>"}`: The address of a PCI function that should
//     be made available to the guest as its network interface.
//   - `"netdev": {"name": "<name>"}`: A pre-existing netdev which should be
//     connected to the network interface of the VM. The provider is responsible
//     for making that connection, e.g. by creating a macvlan tap to bridge
//     between the guest and the host side.
//
// Any prefixes or addresses returned by the plugin are the effective state and
// should be reported to the machinepoollet via IRI.
//
// For delete the plugin is called with INI_COMMAND=DEL and the following
// configuration:
//
//	{"name": "<name>"}
//
// The plugin is expected to not produce output on stdout, unless it encountered
// an error.
package ini

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/netip"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	machinepoolletv1alpha1 "github.com/ironcore-dev/ironcore/poollet/machinepoollet/api/v1alpha1"
	"github.com/ironcore-dev/libvirt-provider/api"
	providerhost "github.com/ironcore-dev/libvirt-provider/internal/host"
	providernetworkinterface "github.com/ironcore-dev/libvirt-provider/internal/plugins/networkinterface"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	PluginName = "ini"

	execTimeout = 30 * time.Second

	perm = 0o777

	envCommand = "INI_COMMAND"

	commandAdd = "ADD"
	commandDel = "DEL"
)

type request struct {
	Name string `json:"name"`
	// Hostname is the hostname the plugin should assign the guest. It is only
	// sent with ADD and empty if the runtime cannot derive a sensible
	// hostname, in which case the plugin applies its own default.
	Hostname string `json:"hostname,omitempty"`
}

type addResult struct {
	Tap      *tapDevice `json:"tap"`
	Prefixes []string   `json:"prefixes"`
	Ips      []string   `json:"ips"`
}

type tapDevice struct {
	Name string `json:"name"`
}

type errorResult struct {
	Error string `json:"error"`
}

type Plugin struct {
	pluginPath string
	host       providerhost.LibvirtHost
}

func NewPlugin(pluginPath string) (providernetworkinterface.Plugin, error) {
	if pluginPath == "" {
		return nil, fmt.Errorf("no ini plugin path provided")
	}
	return &Plugin{pluginPath: pluginPath}, nil
}

func (p *Plugin) Name() string {
	return PluginName
}

func (p *Plugin) Init(_ context.Context, host providerhost.LibvirtHost) error {
	p.host = host
	return nil
}

func (p *Plugin) AddEventHandler(providernetworkinterface.EventHandler) {}

func AttachmentName(machineID, networkInterfaceName string) string {
	return machineID + "-" + networkInterfaceName
}

func (p *Plugin) Apply(ctx context.Context, spec *api.NetworkInterfaceSpec, machine *api.Machine) (*providernetworkinterface.NetworkInterface, error) {
	log := ctrl.LoggerFrom(ctx)

	if err := os.MkdirAll(p.host.MachineNetworkInterfaceDir(machine.ID, spec.Name), perm); err != nil {
		return nil, err
	}

	name := AttachmentName(machine.ID, spec.Name)
	res := &addResult{}
	if err := p.run(ctx, commandAdd, request{Name: name, Hostname: hostname(machine)}, res); err != nil {
		return nil, err
	}

	var prefixes []string
	for _, p := range res.Prefixes {
		prefix, err := netip.ParsePrefix(p)
		if err != nil {
			return nil, fmt.Errorf("ini plugin ADD for attachment %q returned invalid prefix %q: %w",
				name, p, err)
		}
		prefixes = append(prefixes, prefix.String())
	}

	var ips []string
	for _, i := range res.Ips {
		ip, err := netip.ParseAddr(i)
		if err != nil {
			return nil, fmt.Errorf("ini plugin ADD for attachment %q returned invalid ip %q: %w",
				name, i, err)
		}
		ips = append(ips, ip.String())
	}

	nic := &providernetworkinterface.NetworkInterface{
		Handle:   name,
		Prefixes: prefixes,
		Ips:      ips,
	}
	switch {
	case res.Tap != nil:
		if res.Tap.Name == "" {
			return nil, fmt.Errorf("ini plugin ADD for attachment %q returned a tap device without a name", name)
		}
		nic.Ethernet = &providernetworkinterface.Ethernet{Dev: res.Tap.Name}
		log.V(1).Info("Applied INI attachment", "attachment", name, "tapName", res.Tap.Name, "prefixes", prefixes, "ips", ips)
	default:
		return nil, fmt.Errorf("ini plugin ADD for attachment %q returned no device member", name)
	}
	return nic, nil
}

// hostname derives the hostname to assign the guest. An explicitly
// configured guest hostname takes precedence. Otherwise the name of the
// machine object the user created, which the machinepoollet records as a
// label on the IRI machine, is sanitized into a valid DNS label. An empty
// result means the runtime has no hostname to suggest.
func hostname(machine *api.Machine) string {
	if guestConfig := machine.Spec.GuestConfig; guestConfig != nil && guestConfig.HostName != "" {
		return guestConfig.HostName
	}
	return sanitizeHostname(machine.Labels[machinepoolletv1alpha1.MachineNameLabel])
}

var invalidHostnameChars = regexp.MustCompile(`[^a-z0-9]+`)

// sanitizeHostname converts s into a valid DNS label (RFC 1123): lowercase,
// runs of invalid characters replaced by dashes, no leading or trailing
// dashes and at most 63 characters.
func sanitizeHostname(s string) string {
	s = invalidHostnameChars.ReplaceAllString(strings.ToLower(s), "-")
	s = strings.Trim(s, "-")
	if len(s) > 63 {
		s = strings.TrimRight(s[:63], "-")
	}
	return s
}

func (p *Plugin) Delete(ctx context.Context, computeNicName string, machineID string) error {
	name := AttachmentName(machineID, computeNicName)
	if err := p.run(ctx, commandDel, request{Name: name}, nil); err != nil {
		return err
	}
	return os.RemoveAll(p.host.MachineNetworkInterfaceDir(machineID, computeNicName))
}

// run executes the INI plugin binary once: INI_COMMAND=<command> in the
// environment, req as JSON on stdin. If out is non-nil, stdout is decoded into
// it as JSON (for ADD); otherwise stdout is ignored (for DEL).
func (p *Plugin) run(ctx context.Context, command string, req request, out any) error {
	input, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("error encoding ini plugin request: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, execTimeout)
	defer cancel()

	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, p.pluginPath)
	cmd.Env = append(os.Environ(), envCommand+"="+command)
	cmd.Stdin = bytes.NewReader(input)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// Per spec, a failing plugin SHOULD print {"error": "<message>"} to stdout.
		errResult := &errorResult{}
		if jsonErr := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), errResult); jsonErr == nil && errResult.Error != "" {
			return fmt.Errorf("ini plugin %s for attachment %q failed: %s", command, req.Name, errResult.Error)
		}
		if stderrMsg := strings.TrimSpace(stderr.String()); stderrMsg != "" {
			return fmt.Errorf("ini plugin %s for attachment %q failed: %w (stderr: %s)", command, req.Name, err, stderrMsg)
		}
		return fmt.Errorf("ini plugin %s for attachment %q failed: %w", command, req.Name, err)
	}

	if out == nil {
		return nil
	}
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), out); err != nil {
		return fmt.Errorf("error decoding ini plugin %s result for attachment %q (stdout: %q): %w",
			command, req.Name, strings.TrimSpace(stdout.String()), err)
	}
	return nil
}
