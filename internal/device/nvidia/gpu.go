// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package nvidia

import (
	"fmt"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/go-logr/logr"
	"github.com/ironcore-dev/libvirt-provider/api"
)

type NvidiaPlugin struct {
	log     logr.Logger
	devices map[api.PCIAddress]bool
}

func NewNvidiaPlugin(log logr.Logger) *NvidiaPlugin {
	return &NvidiaPlugin{
		log:     log,
		devices: map[api.PCIAddress]bool{},
	}
}

func (p *NvidiaPlugin) Init() error {
	ret := nvml.Init()
	if ret != nvml.SUCCESS {
		return fmt.Errorf("unable to initialize NVML: %v", nvml.ErrorString(ret))
	}
	defer func() {
		ret := nvml.Shutdown()
		if ret != nvml.SUCCESS {
			p.log.Error(fmt.Errorf("%v", nvml.ErrorString(ret)), "failed to shut down NVML")
		}
	}()

	count, ret := nvml.DeviceGetCount()
	if ret != nvml.SUCCESS {
		return fmt.Errorf("unable to get device count: %v", nvml.ErrorString(ret))
	}

	for i := range count {
		device, ret := nvml.DeviceGetHandleByIndex(i)
		if ret != nvml.SUCCESS {
			return fmt.Errorf("unable to get device at index %d: %v", i, nvml.ErrorString(ret))
		}

		pciInfo, ret := device.GetPciInfo()
		if ret != nvml.SUCCESS {
			return fmt.Errorf("unable to get pci information of device at index %d: %v", i, nvml.ErrorString(ret))
		}

		p.log.V(1).Info("Found device", "pci", pciInfo.BusId)

		pciAddress := api.PCIAddress{
			Domain: uint(pciInfo.Domain),
			Bus:    uint(pciInfo.Bus),
			Slot:   uint(pciInfo.Device),
			//the last element of the BusId is the function
			Function: uint(pciInfo.BusId[31]),
		}
		p.devices[pciAddress] = false
	}

	return nil
}

func (p *NvidiaPlugin) Claim() (*api.PCIAddress, error) {
	for pciAddress, claimed := range p.devices {
		if !claimed {
			p.devices[pciAddress] = true
			return &pciAddress, nil
		}
	}

	return nil, fmt.Errorf("no more device available")
}

func (p *NvidiaPlugin) Release(pciAddress api.PCIAddress) error {
	if _, ok := p.devices[pciAddress]; !ok {
		return fmt.Errorf("device not available")
	}

	p.devices[pciAddress] = false

	return nil
}
