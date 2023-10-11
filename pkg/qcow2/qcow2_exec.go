// Copyright 2022 OnMetal authors
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

package qcow2

import (
	"fmt"
	"os/exec"
	"strconv"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
)

type Exec struct {
}

func (Exec) Create(filename string, opts ...CreateOption) error {
	o := &CreateOptions{}
	o.ApplyOptions(opts)

	if o.SourceFile == "" && o.Size == nil {
		return fmt.Errorf("must specify Size when creating without source file")
	}

	args := []string{"create", "-f", "qcow2"}

	if o.SourceFile != "" {
		args = append(args,
			"-b", o.SourceFile,
			"-F", "raw",
		)
	}

	args = append(args, filename)

	if o.Size != nil {
		args = append(args, strconv.FormatInt(*o.Size, 10))
	}

	res, err := exec.Command("qemu-img", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("error running qemu-img: %s, exit error %w", string(res), err)
	}
	return nil
}

func init() {
	utilruntime.Must(impls.Add("exec", 0, Exec{}))
}
