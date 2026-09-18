// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package raw_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestRaw(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Raw Suite")
}
