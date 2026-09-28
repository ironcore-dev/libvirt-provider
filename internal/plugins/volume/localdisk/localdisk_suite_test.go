// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package localdisk_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestLocalDisk(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "LocalDisk Suite")
}
