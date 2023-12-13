// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"

	ori "github.com/ironcore-dev/ironcore/iri/apis/machine/v1alpha1"
)

func (s *Server) Exec(ctx context.Context, req *ori.ExecRequest) (*ori.ExecResponse, error) {

	return &ori.ExecResponse{}, nil
}