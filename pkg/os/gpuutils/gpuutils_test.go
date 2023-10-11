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
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"libvirt.org/go/libvirtxml"
)

const (
	testVendorID  = "0x0000"
	testClassID   = "0x000000"
	filePerm      = 0644
	testAttribute = "test"
)

var (
	log        logr.Logger
	virtletDir string
)

var _ = Describe("GetGPUAddress", Ordered, func() {
	BeforeAll(func() {
		for i := 0; i < 10; i++ {
			devicePath := filepath.Join(virtletDir, fmt.Sprintf("0000:00:00.%d", i))
			err := os.MkdirAll(devicePath, os.ModePerm)
			Expect(err).ToNot(HaveOccurred(), "error creating test folder")

			vendorFile := filepath.Join(devicePath, vendorAttribute)
			err = os.WriteFile(vendorFile, []byte(testVendorID), filePerm)
			Expect(err).ToNot(HaveOccurred(), "error writing to vendor file")

			classFile := filepath.Join(devicePath, classAttribute)
			err = os.WriteFile(classFile, []byte(testClassID), filePerm)
			Expect(err).ToNot(HaveOccurred(), "error writing to vendor file")
		}
	})
	When("The PCI devices path does not exist", func() {
		It("Should give error", func() {
			_, err := GetGPUAddress(log, "/testPath")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("error reading the provided pciDevicePath"))
		})
	})
	When("The PCI devices path exists", func() {
		It("Should return error if there is no controller with NVIDIA vendor ID", func() {
			_, err := GetGPUAddress(log, virtletDir)
			Expect(err).To(HaveOccurred())
			Expect(err).Should(MatchError(errNoNVIDIAGPUController))
		})

		It("Should return error if the controller with NVIDIA vendor ID exists but its class ID does not match", func() {
			devicePath := filepath.Join(virtletDir, "0000:vi:00.0")
			err := os.MkdirAll(devicePath, os.ModePerm)
			Expect(err).ToNot(HaveOccurred(), "error creating test folder")

			vendorFile := filepath.Join(devicePath, vendorAttribute)
			err = os.WriteFile(vendorFile, []byte(nvidiaVendorID), filePerm)
			Expect(err).ToNot(HaveOccurred(), "error writing to vendor file")

			classFile := filepath.Join(devicePath, classAttribute)
			err = os.WriteFile(classFile, []byte(testClassID), filePerm)
			Expect(err).ToNot(HaveOccurred(), "error writing to class file")

			_, err = GetGPUAddress(log, virtletDir)
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(errNoNVIDIAGPUController))
		})

		It("Should return error if the controller with NVIDIA class ID exists but its vendor ID does not match", func() {
			devicePath := filepath.Join(virtletDir, "0000:ci:00.0")
			err := os.MkdirAll(devicePath, os.ModePerm)
			Expect(err).ToNot(HaveOccurred(), "error creating test folder")

			vendorFile := filepath.Join(devicePath, vendorAttribute)
			err = os.WriteFile(vendorFile, []byte(testVendorID), filePerm)
			Expect(err).ToNot(HaveOccurred(), "error writing to vendor file")

			classFile := filepath.Join(devicePath, classAttribute)
			err = os.WriteFile(classFile, []byte(fmt.Sprintf("%s0000", controllerClassIdPrefix)), filePerm)
			Expect(err).ToNot(HaveOccurred(), "error writing to class file")

			_, err = GetGPUAddress(log, virtletDir)
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(errNoNVIDIAGPUController))
		})

		It("Should return the desired DomainAddressPCI if the controller with NVIDIA vendor ID and class ID exists", func() {
			devicePath := filepath.Join(virtletDir, "0000:ca:00.0")
			err := os.MkdirAll(devicePath, os.ModePerm)
			Expect(err).ToNot(HaveOccurred(), "error creating test folder")

			vendorFile := filepath.Join(devicePath, vendorAttribute)
			err = os.WriteFile(vendorFile, []byte(nvidiaVendorID), filePerm)
			Expect(err).ToNot(HaveOccurred(), "error writing to vendor file")

			classFile := filepath.Join(devicePath, classAttribute)
			err = os.WriteFile(classFile, []byte(fmt.Sprintf("%s0000", controllerClassIdPrefix)), filePerm)
			Expect(err).ToNot(HaveOccurred(), "error writing to vendor file")

			domainAddressPCI, err := GetGPUAddress(log, virtletDir)
			Expect(err).NotTo(HaveOccurred())

			expectedDomain := uint(0)
			expectedBus := uint(202) // 0xca
			exptectedSlot := uint(0)
			exptedFunction := uint(0)
			Expect(domainAddressPCI).To(Equal(&libvirtxml.DomainAddressPCI{
				Domain:   &expectedDomain,
				Bus:      &expectedBus,
				Slot:     &exptectedSlot,
				Function: &exptedFunction,
			}))
		})
	})
})

var _ = DescribeTable("parseHexStringToUint",
	func(input string, expectedValue uint, expectError bool) {
		result, err := parseHexStringToUint(input)

		if expectError {
			Expect(err).To(HaveOccurred())
			Expect(result).To(BeNil())
		} else {
			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil())
			Expect(*result).To(Equal(expectedValue))
		}
	},
	Entry("with a valid hexadecimal string", "1A", uint(26), false),
	Entry("with an invalid hexadecimal string", "invalid_hex", uint(0), true),
	Entry("with a very large hexadecimal string", "FFFFFFFFF", uint(0), true),
)

var _ = DescribeTable("gpuAddressToDomainAddressPCI",
	func(domain, bus, slot, function string, expectedErrorMsg string) {
		result, err := gpuAddressToDomainAddressPCI(domain, bus, slot, function)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(expectedErrorMsg))
		Expect(result).To(BeNil())
	},
	Entry("should return an error for invalid domain", "invalid", "00", "00", "0", "error parsing domain to uint"),
	Entry("should return an error for invalid bus", "0000", "invalid", "00", "0", "error parsing bus to uint"),
	Entry("should return an error for invalid slot", "0000", "00", "invalid", "0", "error parsing slot to uint"),
	Entry("should return an error for invalid function", "0000", "00", "00", "invalid", "error parsing function to uint"),
)

var _ = DescribeTable("parsePCIAddress",
	func(input, expectedErrorMsg string) {
		domain, bus, slot, function, err := parsePCIAddress(input)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(expectedErrorMsg))
		Expect(domain).To(BeEmpty())
		Expect(bus).To(BeEmpty())
		Expect(slot).To(BeEmpty())
		Expect(function).To(BeEmpty())
	},
	Entry("should return an error", "invalid_address", "error parsing PCI address"),
)

var _ = DescribeTable("readPCIAttributeWithBufio",
	func(log logr.Logger, virtletDir, testAttribute string) {
		_, err := readPCIAttributeWithBufio(log, virtletDir, testAttribute)
		Expect(err).To(HaveOccurred())
		Expect(errors.Is(err, os.ErrNotExist)).To(BeTrue())
	},
	Entry("should return an error", log, virtletDir, testAttribute),
)
