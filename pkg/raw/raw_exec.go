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

package raw

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
	var seek string

	if o.SourceFile == "" && o.Size == nil {
		return fmt.Errorf("must specify Size when creating without source file")
	}

	ofilename := "of=" + filename
	if o.Size != nil {
		seek = "seek=" + strconv.FormatInt(*o.Size, 10) // TODO: verify if the disk size is proper, else G as a suffix
	}
	if o.SourceFile == "" {
		outMsgDD, err := exec.Command("dd", "if=/dev/zero", ofilename, "bs=1", "count=0", seek).Output()
		if err != nil {
			return fmt.Errorf("failed creating the ephemeral disk: %s, exit error %w", string(outMsgDD), err)
		}
	} else {
		ifilename := "if=" + o.SourceFile
		outMsgDD, err := exec.Command("dd", ifilename, ofilename).Output()
		if err != nil {
			return fmt.Errorf("failed creating the image for virtual disk: %s, exit error %w", string(outMsgDD), err)
		}
	}

	return nil
}

func init() {
	utilruntime.Must(impls.Add("exec", 0, Exec{}))
}
