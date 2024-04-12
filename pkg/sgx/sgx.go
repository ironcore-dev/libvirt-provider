// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package sgx

import (
	"fmt"
	"strings"

	"github.com/ironcore-dev/libvirt-provider/pkg/api"
	"libvirt.org/go/libvirtxml"
)

func EnableSGXInDomain(machineSpec *api.MachineSpec, domain *libvirtxml.Domain) {
	key, quantity, found := FindSGXNumaResource(machineSpec.Resources)
	if !found {
		return
	}

	if domain.QEMUCommandline == nil {
		domain.QEMUCommandline = &libvirtxml.DomainQEMUCommandline{}
	}

	domain.QEMUCommandline.Args = append(domain.QEMUCommandline.Args,
		libvirtxml.DomainQEMUCommandlineArg{
			Value: "-cpu",
		},
		libvirtxml.DomainQEMUCommandlineArg{
			Value: "host,+sgx,+sgx2",
		},
		// Define one EPC section with memoryEPCQuantity.Value() pre-allocated to the guest.
		// EPC size must be defined based on the binarySI format.
		// Defining EPC size as bytes, analog to the defined memory of the guest.
		// Note: EPC size is pre-allocated additionally to the specified memory.
		libvirtxml.DomainQEMUCommandlineArg{
			Value: "-object",
		},
		libvirtxml.DomainQEMUCommandlineArg{
			Value: fmt.Sprintf("memory-backend-epc,id=mem1,size=%dB,prealloc=on", quantity.Value()),
		},
		libvirtxml.DomainQEMUCommandlineArg{
			Value: "-M",
		},
		libvirtxml.DomainQEMUCommandlineArg{
			Value: fmt.Sprintf("sgx-epc.0.memdev=mem1,sgx-epc.0.node=%s", strings.TrimPrefix(string(key), string(ResourceMemorySGXNumaPrefix))),
		})
}
