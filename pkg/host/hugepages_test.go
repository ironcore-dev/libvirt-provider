// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package host_test

import (
	"context"

	"github.com/go-logr/logr"
	. "github.com/ironcore-dev/libvirt-provider/pkg/host"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"libvirt.org/go/libvirtxml"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var _ = Describe("Testing Hugepages", func() {
	Context("Hugepage tuner", func() {
		domain := &libvirtxml.Domain{
			Name: "NumaDomain",
			Memory: &libvirtxml.DomainMemory{
				Value: 10 * 1024 * 1024 * 1024, // 10Gi
				Unit:  Byte,
			},
			VCPU: &libvirtxml.DomainVCPU{Value: 2},
		}
		It("Should tune the numa tuner", func() {
			ctx := logr.NewContext(context.Background(), zap.New())
			n := &HugepageTuner{
				Lv:           lv,
				HugePageSize: hugePageSize,
			}
			Expect(n.Tune(ctx, domain, nil, nil)).To(Succeed())
		})
		It("Should return expected memory backing", func() {
			Expect(domain.MemoryBacking).To(Equal(&libvirtxml.DomainMemoryBacking{
				MemoryHugePages: &libvirtxml.DomainMemoryHugepages{},
			}))
		})
	})
})
