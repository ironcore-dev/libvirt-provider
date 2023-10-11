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

package host

import (
	"testing"

	"github.com/digitalocean/go-libvirt"
	mockdialer "github.com/onmetal/virtlet/mocks/libvirt_dialer"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	dialer *mockdialer.MockLibvirtDilaer
	lv     *libvirt.Libvirt
	zero   uint = 0
)

const hugePageSize = 1048576

func TestHost(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Host Suite")
}

var _ = BeforeSuite(func() {
	domainsList := []libvirt.Domain{
		{
			Name: "Domain1",
			UUID: mockdialer.NewUUID(),
			ID:   1,
		},
	}

	dialer = mockdialer.NewMockDialer(domainsList)
	lv = libvirt.NewWithDialer(dialer)
	Expect(lv.Connect()).To(Succeed())
})

var _ = AfterSuite(func() {
	Expect(lv.ConnectClose()).To(Succeed())
	Expect(dialer.Close()).To(Succeed())
})
