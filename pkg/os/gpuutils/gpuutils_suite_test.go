// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package gpuutils

import (
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestGPUUtils(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "GPUUtils Suite")
}

var _ = BeforeSuite(func() {
	var err error
	providerDir, err = os.MkdirTemp("", "libvirt-provider-GPUUtils-test-")
	Expect(err).ToNot(HaveOccurred(), "error creating temporary directory for gpu test")
})

var _ = AfterSuite(func() {
	err := os.RemoveAll(providerDir)
	Expect(err).ToNot(HaveOccurred(), "error cleanup test folder")
})
