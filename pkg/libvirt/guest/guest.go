// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package guest

import (
	"encoding/xml"
	"fmt"
	"regexp"
	"strconv"

	"github.com/digitalocean/go-libvirt"
	"libvirt.org/go/libvirtxml"
)

type OSType string

const (
	OSTypeHVM OSType = "hvm"
)

type Requests struct {
	Architecture string
	OSType       OSType
}

type Settings struct {
	Type    string
	Machine string
}

type Capabilities interface {
	SettingsFor(reqs Requests) (*Settings, error)
}

type capabilties struct {
	caps []libvirtxml.CapsGuest

	preferredDomainTypes  []string
	preferredMachineTypes []string
}

func (c *capabilties) archDomainType(arch *libvirtxml.CapsGuestArch) (string, int, bool) {
	if len(c.preferredDomainTypes) == 0 {
		if len(arch.Domains) == 0 {
			return "", 0, false
		}
		// TODO: Do we want to allow this?
		return arch.Domains[0].Type, 0, true
	}

	var (
		bestDomainType         string
		bestDomainTypePriority = -1
	)
	for _, domain := range arch.Domains {
		for inversePriority, preferredDomainType := range c.preferredDomainTypes {
			priority := len(c.preferredDomainTypes) - inversePriority
			if priority <= bestDomainTypePriority {
				continue
			}

			if domain.Type != preferredDomainType {
				continue
			}

			bestDomainType = domain.Type
			bestDomainTypePriority = inversePriority
			break
		}

		if bestDomainTypePriority == len(c.preferredDomainTypes) {
			break
		}
	}

	return bestDomainType, bestDomainTypePriority, bestDomainType != ""
}

func (c *capabilties) archMachineType(arch *libvirtxml.CapsGuestArch) (string, int, bool) {
	if len(c.preferredMachineTypes) == 0 {
		if len(arch.Machines) == 0 {
			return "", 0, false
		}
		// TODO: Do we want to allow this?
		return arch.Machines[0].Name, 0, true
	}

	var (
		bestMachineType         string
		bestMachineTypeVersion  *MachineTypeVersion
		bestMachineTypePriority = -1
	)
	for _, machine := range arch.Machines {
		for inversePriority, preferredMachineType := range c.preferredMachineTypes {
			priority := len(c.preferredMachineTypes) - inversePriority
			if priority < bestMachineTypePriority {
				continue
			}

			machineType, machineTypeVersion := ParseMachineTypeVersion(machine.Name, machine.Canonical)
			if machineType != preferredMachineType {
				continue
			}

			// In case we're having two machines of the same type but different versions, use the later version.
			if bestMachineType == machineType && (bestMachineTypeVersion != nil && (machineTypeVersion == nil || machineTypeVersion.Compare(bestMachineTypeVersion) <= 0)) {
				continue
			}

			bestMachineType = machineType
			bestMachineTypeVersion = machineTypeVersion
			bestMachineTypePriority = priority
			break
		}
	}
	return JoinMachineTypeVersion(bestMachineType, bestMachineTypeVersion), bestMachineTypePriority, bestMachineType != ""
}

var machineTypeVersionRegex = regexp.MustCompile(`^([a-zA-Z0-9-]+)-([0-9]+)\.([0-9]+)$`)

func ParseMachineTypeVersion(machineType, canonical string) (string, *MachineTypeVersion) {
	if canonical != "" {
		machineType = canonical
	}

	match := machineTypeVersionRegex.FindStringSubmatch(machineType)
	if match == nil {
		return machineType, nil
	}

	name := match[1]
	majorString := match[2]
	minorString := match[3]

	major, _ := strconv.Atoi(majorString)
	minor, _ := strconv.Atoi(minorString)
	return name, &MachineTypeVersion{major, minor}
}

func JoinMachineTypeVersion(name string, version *MachineTypeVersion) string {
	if version == nil {
		return name
	}
	return fmt.Sprintf("%s-%s", name, version)
}

type MachineTypeVersion struct {
	Major int
	Minor int
}

func (m *MachineTypeVersion) Compare(other *MachineTypeVersion) int {
	if n := m.Major - other.Major; n != 0 {
		return n
	}
	return m.Minor - other.Minor
}

func (m *MachineTypeVersion) String() string {
	return fmt.Sprintf("%d.%d", m.Major, m.Minor)
}

func (c *capabilties) SettingsFor(reqs Requests) (*Settings, error) {
	if reqs.Architecture == "" {
		return nil, fmt.Errorf("must specify Requests.Architecture")
	}
	if reqs.OSType == "" {
		return nil, fmt.Errorf("must specify Requests.OSType")
	}

	var (
		bestDomainType      string
		bestDomainTypePrio  = -1
		bestMachineType     string
		bestMachineTypePrio = -1
	)
	for _, capability := range c.caps {
		if capability.OSType != string(reqs.OSType) {
			continue
		}

		arch := capability.Arch
		if arch.Name != reqs.Architecture {
			continue
		}

		domainType, domainTypePrio, ok := c.archDomainType(&arch)
		if !ok || domainTypePrio <= bestDomainTypePrio {
			continue
		}

		machineType, machineTypePrio, ok := c.archMachineType(&arch)
		if !ok || machineTypePrio < bestMachineTypePrio {
			continue
		}

		bestDomainType = domainType
		bestDomainTypePrio = domainTypePrio
		bestMachineType = machineType
		bestMachineTypePrio = machineTypePrio

		if bestDomainTypePrio == len(c.preferredDomainTypes) && bestMachineTypePrio == len(c.preferredMachineTypes) {
			break
		}
	}

	if bestDomainType != "" && bestMachineType != "" {
		return &Settings{
			Type:    bestDomainType,
			Machine: bestMachineType,
		}, nil
	}
	return nil, fmt.Errorf("no matching settings for requests %#+v", reqs)
}

type CapabilitiesOptions struct {
	PreferredMachineTypes []string
	PreferredDomainTypes  []string
}

// DetectCapabilities
func DetectCapabilities(lv *libvirt.Libvirt, opts CapabilitiesOptions) (Capabilities, error) {
	capsData, err := lv.Capabilities()
	if err != nil {
		return nil, fmt.Errorf("error getting capabilities: %w", err)
	}

	var caps libvirtxml.Caps
	if err := xml.Unmarshal(capsData, &caps); err != nil {
		return nil, fmt.Errorf("error unmarshalling guest capabilities: %w", err)
	}

	return &capabilties{
		caps:                  caps.Guests,
		preferredDomainTypes:  opts.PreferredDomainTypes,
		preferredMachineTypes: opts.PreferredMachineTypes,
	}, nil
}
