// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package host

import (
	"context"
	"encoding/xml"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/digitalocean/go-libvirt"
	"github.com/go-logr/logr"
	"github.com/ironcore-dev/libvirt-provider/pkg/api"
	"github.com/ironcore-dev/libvirt-provider/pkg/controllers"
	"k8s.io/apimachinery/pkg/util/sets"
	"libvirt.org/go/libvirtxml"
)

const (
	KiB  = "KiB"
	Byte = "Byte"
)

type NumaTuner struct {
	Lv           *libvirt.Libvirt
	HugePageSize uint64
	CPUTopology  map[int][]int
}

// CleanBlockedCPUs removes the set of blockedCPUs from the CPUTopology so that Machines are not scheduled on any of the blockedCPUs.
func CleanBlockedCPUs(topology map[int][]int, blockedCPUs []int) map[int][]int {
	if len(blockedCPUs) == 0 {
		return topology
	}

	blockedSet := sets.New[int]()
	blockedSet.Insert(blockedCPUs...)
	for n, cpus := range topology {
		nodeCPUs := sets.New[int]()
		nodeCPUs.Insert(cpus...)

		topology[n] = sets.List(nodeCPUs.Difference(blockedSet))
	}

	return topology
}

func InitNumaTuneFunc(lv *libvirt.Libvirt, HugePageSize uint64, blockedCPU []int) (controllers.TuneFunc, error) {
	cpuTopology, err := CpuTopology(lv)
	if err != nil {
		return nil, fmt.Errorf("error while fetching the CPU Topology from the host: %w", err)
	}
	cpuTopology = CleanBlockedCPUs(cpuTopology, blockedCPU)
	nt := NumaTuner{
		Lv:           lv,
		HugePageSize: HugePageSize,
		CPUTopology:  cpuTopology,
	}
	return nt.Tune, nil
}

func (n *NumaTuner) Tune(ctx context.Context, domain *libvirtxml.Domain, _ *api.Machine, _ *controllers.MachineReconciler) error {
	log, err := logr.FromContext(ctx)
	if err != nil {
		return fmt.Errorf("can't get logger from context: %w", err)
	}

	if domain.VCPU == nil || domain.Memory == nil {
		return nil
	}

	// TODO: add supporting NUMA-aware pinning without hugepages
	if domain.MemoryBacking == nil || domain.MemoryBacking.MemoryHugePages == nil {
		return nil
	}
	requiredMemory := uint64(domain.Memory.Value)

	if domain.Memory.Unit != Byte {
		return fmt.Errorf("unsupported memory unit: %v, please set in bytes", domain.Memory.Unit)
	}

	pins, err := VMCPUPins(n.Lv)
	if err != nil {
		return err
	}

	log.V(1).Info("Numa aware and hugepage memory tuning", "requiredMemory", requiredMemory, "domain", domain.Name)
	if ok, err := n.isEnoughFreePagesAllocated(requiredMemory); !ok || err != nil {
		if !ok {
			return fmt.Errorf("not enough  memory in  preallocated amount of hugepages ")
		}
		return fmt.Errorf("unable to get free pages info: %w", err)
	}

	allocatableMemoryPerNode, err := n.getFreeNumaNodes(log, requiredMemory)
	if err != nil {
		return err
	}

	nodeCPUs := n.CPUTopology
	for node, cpus := range nodeCPUs {
		//	CPU cores with less pinned vm count go first
		sort.SliceStable(cpus, func(i, j int) bool {
			return pins[cpus[i]] < pins[cpus[j]]
		})
		nodeCPUs[node] = cpus
	}

	var totalMemory uint64
	nodeIDs := make([]int, 0, len(allocatableMemoryPerNode))
	for node, mem := range allocatableMemoryPerNode {
		totalMemory += mem
		nodeIDs = append(nodeIDs, node)
	}

	// nodes with more free memory go first
	sort.SliceStable(nodeIDs, func(i, j int) bool {
		return allocatableMemoryPerNode[i] > allocatableMemoryPerNode[j]
	})

	cpuCount := domain.VCPU.Value
	numaCfg := &libvirtxml.DomainNuma{}
	memNodesCfg := make([]libvirtxml.DomainNUMATuneMemNode, 0, len(allocatableMemoryPerNode))
	vcpuPins := make([]libvirtxml.DomainCPUTuneVCPUPin, 0, cpuCount)

	var (
		cellID     uint
		pinnedCPUs uint
		nodeSet    []string
		i          int
	)

	for _, nodeID := range nodeIDs {
		mem := allocatableMemoryPerNode[nodeID]
		var CPUs string

		if pinnedCPUs < cpuCount {
			k := float64(uint64(cpuCount)*mem) / float64(totalMemory)
			s := uint(math.Ceil(k))

			if cpuCount < pinnedCPUs+s {
				s = cpuCount - pinnedCPUs
			}
			pinnedCPUs += s
			selectedCPUS := nodeCPUs[nodeID][:s]

			for _, c := range selectedCPUS {
				vcpuPins = append(vcpuPins, libvirtxml.DomainCPUTuneVCPUPin{VCPU: uint(i), CPUSet: strconv.Itoa(c)})
				CPUs += strconv.Itoa(i) + ","
				i += 1
			}
			CPUs = strings.TrimRight(CPUs, ",") //nolint:staticcheck
		}

		nodeStr := strconv.Itoa(nodeID)

		tmpCellID := cellID
		cell := libvirtxml.DomainCell{
			ID:     &tmpCellID,
			Memory: uint(mem),
			Unit:   KiB,
		}
		if CPUs != "" {
			cell.CPUs = CPUs
		}

		nodeSet = append(nodeSet, nodeStr)
		numaCfg.Cell = append(numaCfg.Cell, cell)
		memNodesCfg = append(memNodesCfg, libvirtxml.DomainNUMATuneMemNode{
			CellID:  tmpCellID,
			Mode:    "strict",
			Nodeset: nodeStr,
		})

		cellID += 1
	}

	numaTuneCfg := &libvirtxml.DomainNUMATune{
		Memory: &libvirtxml.DomainNUMATuneMemory{
			Mode:    "strict",
			Nodeset: strings.Join(nodeSet, ","),
		},
		MemNodes: memNodesCfg,
	}
	domain.NUMATune = numaTuneCfg
	if domain.CPU == nil {
		domain.CPU = &libvirtxml.DomainCPU{}
	}
	domain.CPU.Numa = numaCfg

	if domain.CPUTune == nil {
		domain.CPUTune = &libvirtxml.DomainCPUTune{}
	}

	domain.CPUTune.VCPUPin = vcpuPins

	return nil
}

type numaNodeInfo struct {
	nodeID    int
	freePages uint64
}

// getFreeNumaNodes calculate how much memory and in which nodes we are going to allocate
func (n *NumaTuner) getFreeNumaNodes(log logr.Logger, memory uint64) (map[int]uint64, error) {
	// Align memory to Kb
	needMemoryKb, _ := AlignMemory(memory, 1024)
	needMemoryKb /= 1024

	numaFreePages, err := n.freePages()
	if err != nil {
		return nil, err
	}

	// Create a slice of numaNodeInfo to store both NodeID and FreePages
	nodes := make([]numaNodeInfo, 0, len(numaFreePages))
	for k, freePages := range numaFreePages {
		nodes = append(nodes, numaNodeInfo{nodeID: k, freePages: freePages[n.HugePageSize]})
	}

	// Sort the nodes based on both FreePages and NodeID
	sort.SliceStable(nodes, func(i, j int) bool {
		if nodes[i].freePages != nodes[j].freePages {
			return nodes[i].freePages > nodes[j].freePages
		}
		return nodes[i].nodeID < nodes[j].nodeID
	})

	allocNodes := make(map[int]uint64)
	alignedMemory, _ := AlignMemory(needMemoryKb, n.HugePageSize)
	if alignedMemory > memory {
		log.V(0).Info("Aligned memory size is more than required")
	}

	for _, node := range nodes {
		nodeID := node.nodeID
		freeMem := node.freePages * n.HugePageSize
		if freeMem >= alignedMemory {
			allocNodes[nodeID] = alignedMemory
			alignedMemory = 0
			break
		}
		allocNodes[nodeID] = freeMem
		alignedMemory -= freeMem
	}

	if alignedMemory > 0 {
		return nil, fmt.Errorf("not enough count of free numa pages")
	}

	return allocNodes, nil
}

func (n *NumaTuner) isEnoughFreePagesAllocated(memory uint64) (bool, error) {
	needMemoryKb, _ := AlignMemory(memory, 1024)
	needMemoryKb /= 1024

	numaFreePages, err := n.freePages()
	if err != nil {
		return false, err
	}

	var totalFreeMemKb uint64
	for _, nodePages := range numaFreePages {
		totalFreeMemKb += nodePages[n.HugePageSize] * n.HugePageSize
	}

	return totalFreeMemKb >= needMemoryKb, nil
}

// freePages return the number of free pages of each size for each node
func (n *NumaTuner) freePages() (map[int]map[uint64]uint64, error) {
	capsData, err := n.Lv.Capabilities()
	if err != nil {
		return nil, fmt.Errorf("error getting capabilities: %w", err)
	}

	var caps libvirtxml.Caps
	if err := xml.Unmarshal(capsData, &caps); err != nil {
		return nil, fmt.Errorf("error unmarshalling guest capabilities: %w", err)
	}
	if caps.Host.NUMA == nil || caps.Host.NUMA.Cells == nil || len(caps.Host.NUMA.Cells.Cells) == 0 {
		return nil, fmt.Errorf("no NUMA  node detected on host")
	}

	pages := make([]uint32, 0, len(caps.Host.NUMA.Cells.Cells[0].PageInfo))

	for _, page := range caps.Host.NUMA.Cells.Cells[0].PageInfo {
		if page.Unit != KiB {
			return nil, fmt.Errorf("supporting working only with page sizes in KiB but page in %v given", page.Unit)
		}
		pages = append(pages, uint32(page.Size))
	}
	rCount, err := n.Lv.NodeGetFreePages(pages, 0, uint32(caps.Host.NUMA.Cells.Num), 0)
	if err != nil {
		return nil, err
	}

	freeNodePages := make(map[int]map[uint64]uint64, caps.Host.NUMA.Cells.Num)
	for i := 0; i < int(caps.Host.NUMA.Cells.Num); i++ {
		freePages := make(map[uint64]uint64, len(pages))
		for j, p := range pages {
			freePages[uint64(p)] = rCount[i*len(pages)+j]
		}
		freeNodePages[i] = freePages
	}
	return freeNodePages, nil
}
