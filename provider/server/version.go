// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"

	"github.com/blang/semver/v4"
	ori "github.com/ironcore-dev/ironcore/iri/apis/machine/v1alpha1"
	"github.com/ironcore-dev/libvirt-provider/provider/version"
)

func (s *Server) Version(ctx context.Context, req *ori.VersionRequest) (*ori.VersionResponse, error) {
	var runtimeVersion string
	switch {
	case version.Version != "":
		runtimeVersion = version.Version
	case version.Commit != "":
		v, err := semver.NewBuildVersion(version.Commit)
		if err != nil {
			runtimeVersion = "0.0.0"
		} else {
			runtimeVersion = v
		}
	default:
		runtimeVersion = "0.0.0"
	}

	return &ori.VersionResponse{
		RuntimeName:    version.RuntimeName,
		RuntimeVersion: runtimeVersion,
	}, nil
}
