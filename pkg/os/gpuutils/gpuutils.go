// Copyright 2023 OnMetal authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package gpuutils

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-logr/logr"
	"libvirt.org/go/libvirtxml"
)

const (
	nvidiaVendorID          = "0x10de"
	controllerClassIdPrefix = "0x03"
	classAttribute          = "class"
	vendorAttribute         = "vendor"
)

var (
	errNoNVIDIAGPUController = errors.New("no NVIDIA GPU controller found on host")
)

func GetGPUAddress(log logr.Logger, pciDevicePath string) (*libvirtxml.DomainAddressPCI, error) {
	dirEntries, err := os.ReadDir(pciDevicePath)
	if err != nil {
		return nil, fmt.Errorf("error reading the provided pciDevicePath: %w", err)
	}

	for _, entry := range dirEntries {
		file := filepath.Join(pciDevicePath, entry.Name())

		domainAddressPci, err := processPCIDevice(log, file)
		if err != nil {
			log.Error(err, "error processing PCI device", "Device", entry.Name())
			continue
		}

		if domainAddressPci != nil {
			return domainAddressPci, nil
		}
	}

	return nil, errNoNVIDIAGPUController
}

func processPCIDevice(log logr.Logger, file string) (*libvirtxml.DomainAddressPCI, error) {
	vendorID, err := readPCIAttributeWithBufio(log, file, vendorAttribute)
	if err != nil {
		return nil, err
	}

	if vendorID != nvidiaVendorID {
		return nil, nil
	}

	classID, err := readPCIAttributeWithBufio(log, file, classAttribute)
	if err != nil {
		return nil, err
	}

	if !strings.HasPrefix(classID, controllerClassIdPrefix) {
		return nil, nil
	}

	domain, bus, slot, function, err := parsePCIAddress(filepath.Base(file))
	if err != nil {
		return nil, err
	}

	return gpuAddressToDomainAddressPCI(domain, bus, slot, function)
}

func readPCIAttributeWithBufio(log logr.Logger, devicePath, attributeName string) (string, error) {
	attributePath := filepath.Join(devicePath, attributeName)
	file, err := os.Open(attributePath)
	if err != nil {
		return "", err
	}
	defer func() {
		defErr := file.Close()
		if defErr != nil {
			log.Error(defErr, "error closing file", "Path", file)
		}
	}()

	scanner := bufio.NewScanner(file)
	scanner.Scan()
	if err := scanner.Err(); err != nil {
		return "", err
	}

	return strings.TrimSpace(scanner.Text()), nil
}

func parsePCIAddress(address string) (domain, bus, slot, function string, err error) {
	_, err = fmt.Sscanf(address, "%4s:%2s:%2s.%1s", &domain, &bus, &slot, &function)
	if err != nil {
		return "", "", "", "", fmt.Errorf("error parsing PCI address: %w", err)
	}

	return
}

func gpuAddressToDomainAddressPCI(domainStr, busStr, slotStr, functionStr string) (*libvirtxml.DomainAddressPCI, error) {
	domain, err := parseHexStringToUint(domainStr)
	if err != nil {
		return nil, fmt.Errorf("error parsing domain to uint: %w", err)
	}

	bus, err := parseHexStringToUint(busStr)
	if err != nil {
		return nil, fmt.Errorf("error parsing bus to uint: %w", err)
	}

	slot, err := parseHexStringToUint(slotStr)
	if err != nil {
		return nil, fmt.Errorf("error parsing slot to uint: %w", err)
	}

	function, err := parseHexStringToUint(functionStr)
	if err != nil {
		return nil, fmt.Errorf("error parsing function to uint: %w", err)
	}

	return &libvirtxml.DomainAddressPCI{
		Domain:   domain,
		Bus:      bus,
		Slot:     slot,
		Function: function,
	}, nil
}

func parseHexStringToUint(hexStr string) (*uint, error) {
	hexValue, err := strconv.ParseUint(hexStr, 16, 32) // Assuming 32-bit uint
	if err != nil {
		return nil, err
	}
	uintValue := uint(hexValue)
	return &uintValue, nil
}
