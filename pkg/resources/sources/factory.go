// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package sources

import (
	"fmt"

	"github.com/ironcore-dev/libvirt-provider/pkg/resources/manager"
)

func GetSource(name string) (manager.Source, error) {
	switch name {
	case "memory":
		return NewSourceMemory(), nil
	case "cpu":
		return NewSourceCPU(), nil
	case "hugepages":
		return NewSourceHugepages(), nil
	default:
		return nil, fmt.Errorf("unsupported source %s", name)
	}
}

func GetSourcesAvailable() []string {
	return []string{"memory", "cpu", "hugepages"}
}
