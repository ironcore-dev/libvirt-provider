// Copyright 2023 OnMetal authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package host_test

import (
	"testing"
	"time"

	"github.com/onmetal/libvirt-driver/pkg/api"
	"github.com/onmetal/libvirt-driver/pkg/host"
	"github.com/onmetal/libvirt-driver/pkg/store"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	tmpDir       string
	machineStore store.Store[*api.Machine]
)

const (
	eventuallyTimeout    = 5 * time.Second
	pollingInterval      = 250 * time.Millisecond
	consistentlyDuration = 1 * time.Second
)

func TestServer(t *testing.T) {
	SetDefaultConsistentlyPollingInterval(pollingInterval)
	SetDefaultEventuallyPollingInterval(pollingInterval)
	SetDefaultEventuallyTimeout(eventuallyTimeout)
	SetDefaultConsistentlyDuration(consistentlyDuration)

	RegisterFailHandler(Fail)
	RunSpecs(t, "Server Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	var err error

	tmpDir = GinkgoT().TempDir()

	machineStore, err = host.NewStore[*api.Machine](host.Options[*api.Machine]{
		Dir: tmpDir,
		NewFunc: func() *api.Machine {
			return &api.Machine{}
		},
	})
	Expect(err).NotTo(HaveOccurred())
})
