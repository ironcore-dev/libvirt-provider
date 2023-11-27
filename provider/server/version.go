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
