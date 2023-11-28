// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

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
