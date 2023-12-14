// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package host

import (
	"encoding/xml"
	"fmt"

	"github.com/digitalocean/go-libvirt"
	"libvirt.org/go/libvirtxml"
)

// CpuTopology return a list of related to NUMA node CPU cores
func CpuTopology(lv *libvirt.Libvirt) (map[int][]int, error) {
	capsData, err := lv.Capabilities()
	if err != nil {
		return nil, fmt.Errorf("error getting capabilities: %w", err)
	}

	var caps libvirtxml.Caps
	if err := xml.Unmarshal(capsData, &caps); err != nil {
		return nil, fmt.Errorf("error unmarshalling guest capabilities: %w", err)
	}
	if caps.Host.NUMA == nil || caps.Host.NUMA.Cells == nil || len(caps.Host.NUMA.Cells.Cells) == 0 {
		return nil, fmt.Errorf("no NUMA node detected on the host")
	}

	node := make(map[int][]int, caps.Host.NUMA.Cells.Num)

	for _, cell := range caps.Host.NUMA.Cells.Cells {
		cpus := make([]int, cell.CPUS.Num)
		for i, c := range cell.CPUS.CPUs {
			cpus[i] = c.ID
		}
		node[cell.ID] = cpus
	}
	return node, nil
}

// getVcpuPinInfo return the CPU affinity setting of all virtual CPUs of domain
func getVcpuPinInfo(lv *libvirt.Libvirt, domain libvirt.Domain) ([][]bool, error) {
	_, _, _, _, rNodes, rSockets, rCores, rThreads, err := lv.NodeGetInfo()
	if err != nil {
		return nil, err
	}
	npcpus := rNodes * rSockets * rCores * rThreads
	maplen := (npcpus + 7) / 8

	_, _, _, rNrVirtCPU, _, err := lv.DomainGetInfo(domain)
	if err != nil {
		return nil, err
	}

	rCpuMaps, _, err := lv.DomainGetVcpuPinInfo(domain, int32(rNrVirtCPU), maplen, 1)
	if err != nil {
		return nil, err
	}
	cpumaps := make([][]bool, rNrVirtCPU)
	for i := 0; i < int(rNrVirtCPU); i++ {
		cpumaps[i] = make([]bool, npcpus)
		for j := 0; j < int(npcpus); j++ {
			b := (i * int(maplen)) + (j / 8)
			bit := j % 8
			if (rCpuMaps[b] & (1 << uint(bit))) != 0 {
				cpumaps[i][j] = true
			}
		}
	}
	return cpumaps, nil
}

// VMCPUPins return count of
func VMCPUPins(lv *libvirt.Libvirt) (map[int]int, error) {
	domains, _, err := lv.ConnectListAllDomains(1, libvirt.ConnectListDomainsActive)
	if err != nil {
		return nil, err
	}
	// calculating how many VM use each core
	pins := make(map[int]int)
	for _, d := range domains {
		p, err := getVcpuPinInfo(lv, d)
		if err != nil {
			return nil, err
		}
		for _, cores := range p {
			for core, pinned := range cores {
				if pinned {
					pins[core] += 1
				}
			}
		}
	}
	return pins, nil
}
