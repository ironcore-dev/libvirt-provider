// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"

	ori "github.com/ironcore-dev/ironcore/iri/apis/machine/v1alpha1"
)

func (s *Server) DetachNetworkInterface(
	ctx context.Context,
	req *ori.DetachNetworkInterfaceRequest,
) (*ori.DetachNetworkInterfaceResponse, error) {

	return &ori.DetachNetworkInterfaceResponse{}, nil
}
