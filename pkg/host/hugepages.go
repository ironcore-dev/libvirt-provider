// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package host

import (
	"context"
	"encoding/xml"
	"fmt"

	"github.com/digitalocean/go-libvirt"
	"github.com/go-logr/logr"
	"github.com/ironcore-dev/libvirt-provider/pkg/api"
	"github.com/ironcore-dev/libvirt-provider/pkg/controllers"
	"libvirt.org/go/libvirtxml"
)

type HugepageTuner struct {
	Lv           *libvirt.Libvirt
	HugePageSize uint64
}

func InitHugepageTuner(lv *libvirt.Libvirt, HugePageSize uint64) (controllers.TuneFunc, error) {
	nt := HugepageTuner{
		Lv:           lv,
		HugePageSize: HugePageSize,
	}
	return nt.Tune, nil
}

func (n *HugepageTuner) Tune(ctx context.Context, domain *libvirtxml.Domain, _ *api.Machine, _ *controllers.MachineReconciler) error {
	log, err := logr.FromContext(ctx)
	if err != nil {
		return fmt.Errorf("can't get logger from context: %w", err)
	}

	if domain.Memory == nil {
		return nil
	}
	requiredMemory := uint64(domain.Memory.Value)

	if domain.Memory.Unit != Byte {
		return fmt.Errorf("unsupported memory unit: %v, please set in bytes", domain.Memory.Unit)
	}

	log.V(1).Info("Hugepage memory tuning", "requiredMemory", requiredMemory, "domain", domain.Name)
	if ok, err := n.isEnoughFreePagesAllocated(requiredMemory); !ok || err != nil {
		if !ok {
			return fmt.Errorf("not enough  memory in preallocated amount of hugepages ")
		}
		return fmt.Errorf("unable to get free pages info: %w", err)
	}

	domain.MemoryBacking = &libvirtxml.DomainMemoryBacking{
		MemoryHugePages: &libvirtxml.DomainMemoryHugepages{},
	}

	return nil
}

func (n *HugepageTuner) isEnoughFreePagesAllocated(memory uint64) (bool, error) {
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
func (n *HugepageTuner) freePages() (map[int]map[uint64]uint64, error) {
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
