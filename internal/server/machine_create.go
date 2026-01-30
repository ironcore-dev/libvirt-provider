// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	corev1alpha1 "github.com/ironcore-dev/ironcore/api/core/v1alpha1"
	iri "github.com/ironcore-dev/ironcore/iri/apis/machine/v1alpha1"
	"github.com/ironcore-dev/libvirt-provider/api"
	apiutils "github.com/ironcore-dev/provider-utils/apiutils/api"
	claim "github.com/ironcore-dev/provider-utils/claimutils/claim"
	"github.com/ironcore-dev/provider-utils/claimutils/gpu"
	"github.com/ironcore-dev/provider-utils/claimutils/pci"
	"k8s.io/apimachinery/pkg/api/resource"
)

func calcResources(class *iri.MachineClass) (int64, int64) {
	//Todo do some magic
	return class.Capabilities.Resources[string(corev1alpha1.ResourceCPU)], class.Capabilities.Resources[string(corev1alpha1.ResourceMemory)]
}

func filterNvidiaGPUResources(capRes map[string]int64) corev1alpha1.ResourceList {
	nvidiaRes := corev1alpha1.ResourceList{}
	if _, ok := capRes[api.NvidiaGPUPlugin]; ok {
		nvidiaRes[api.NvidiaGPUPlugin] = *resource.NewQuantity(capRes[api.NvidiaGPUPlugin], resource.DecimalSI)
	}
	return nvidiaRes
}

func getPCIAddresses(claims claim.Claims) ([]pci.Address, error) {
	if resClaim, ok := claims[api.NvidiaGPUPlugin]; ok {
		gpuClaim, ok := resClaim.(gpu.Claim)
		if !ok {
			return nil, fmt.Errorf("failed to cast GPU claim to gpu.Claim type")
		}
		return gpuClaim.PCIAddresses(), nil
	}

	return []pci.Address{}, nil
}

func (s *Server) createMachineFromIRIMachine(ctx context.Context, log logr.Logger, iriMachine *iri.Machine) (*api.Machine, error) {
	//TODO cleanup: release GPU claims if other errors occur
	log.V(2).Info("Getting libvirt machine config")

	switch {
	case iriMachine == nil:
		return nil, fmt.Errorf("iri machine is nil")
	case iriMachine.Spec == nil:
		return nil, fmt.Errorf("iri machine spec is nil")
	case iriMachine.Metadata == nil:
		return nil, fmt.Errorf("iri machine metadata is nil")
	}

	class, found := s.machineClasses.Get(iriMachine.Spec.Class)
	if !found {
		return nil, fmt.Errorf("machine class '%s' not supported", iriMachine.Spec.Class)
	}
	log.V(2).Info("Validated class")

	cpu, memory := calcResources(class)

	power, err := s.getPowerStateFromIRI(iriMachine.Spec.Power)
	if err != nil {
		return nil, fmt.Errorf("failed to get power state: %w", err)
	}

	var volumes []*api.VolumeSpec
	for _, iriVolume := range iriMachine.Spec.Volumes {
		volumeSpec, err := s.getVolumeFromIRIVolume(iriVolume)
		if err != nil {
			return nil, fmt.Errorf("error converting volume: %w", err)
		}

		volumes = append(volumes, volumeSpec)
	}

	var networkInterfaces []*api.NetworkInterfaceSpec
	for _, iriNetworkInterface := range iriMachine.Spec.NetworkInterfaces {
		networkInterfaceSpec := &api.NetworkInterfaceSpec{
			Name:       iriNetworkInterface.Name,
			NetworkId:  iriNetworkInterface.NetworkId,
			Ips:        iriNetworkInterface.Ips,
			Attributes: iriNetworkInterface.Attributes,
		}
		networkInterfaces = append(networkInterfaces, networkInterfaceSpec)
	}

	gpus, err := s.resourceClaimer.Claim(ctx, filterNvidiaGPUResources(class.Capabilities.Resources))
	if err != nil {
		log.Error(err, "Failed to claim GPUs")
		return nil, fmt.Errorf("failed to claim GPUs: %w", err)
	}

	pciAddrs, err := getPCIAddresses(gpus)
	if err != nil {
		log.Error(err, "Failed to get PCI addresses from GPU claims")
		return nil, fmt.Errorf("failed to get PCI addresses: %w", err)
	}
	log.V(2).Info("Claimed GPU PCI addresses", "pciAddresses", fmt.Sprintf("%v", pciAddrs))

	machine := &api.Machine{
		Metadata: apiutils.Metadata{
			ID: s.idGen.Generate(),
		},
		Spec: api.MachineSpec{
			Power:             power,
			Cpu:               cpu,
			MemoryBytes:       memory,
			Volumes:           volumes,
			Ignition:          iriMachine.Spec.IgnitionData,
			NetworkInterfaces: networkInterfaces,
			Gpu:               pciAddrs,
			GuestAgent:        s.guestAgent,
		},
	}

	if err := api.SetObjectMetadata(machine, iriMachine.Metadata); err != nil {
		return nil, fmt.Errorf("failed to set metadata: %w", err)
	}
	api.SetClassLabel(machine, iriMachine.Spec.Class)
	api.SetManagerLabel(machine, api.MachineManager)

	apiMachine, err := s.machineStore.Create(ctx, machine)
	if err != nil {
		return nil, fmt.Errorf("failed to create machine: %w", err)
	}

	return apiMachine, nil
}

func (s *Server) CreateMachine(ctx context.Context, req *iri.CreateMachineRequest) (res *iri.CreateMachineResponse, retErr error) {
	log := s.loggerFrom(ctx)

	log.V(1).Info("Creating machine from iri machine")
	machine, err := s.createMachineFromIRIMachine(ctx, log, req.Machine)
	if err != nil {
		return nil, convertInternalErrorToGRPC(fmt.Errorf("unable to get libvirt machine config: %w", err))
	}

	log.V(1).Info("Converting machine to iri machine")
	iriMachine, err := s.convertMachineToIRIMachine(ctx, log, machine)
	if err != nil {
		return nil, convertInternalErrorToGRPC(fmt.Errorf("unable to convert machine: %w", err))
	}

	return &iri.CreateMachineResponse{
		Machine: iriMachine,
	}, nil
}
