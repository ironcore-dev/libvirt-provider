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
	virtletDir, err = os.MkdirTemp("", "virtlet-GPUUtils-test-")
	Expect(err).ToNot(HaveOccurred(), "error creating temporary directory for gpu test")
})

var _ = AfterSuite(func() {
	err := os.RemoveAll(virtletDir)
	Expect(err).ToNot(HaveOccurred(), "error cleanup test folder")
})
