// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package sources

import "errors"

const (
	QuantityCountIgnore = -1
)

var (
	ErrResourceNotAvailable = errors.New("not enough available resources")
	ErrResourceMissing      = errors.New("resource is missing")

	ErrSourceResourceUnsupport = errors.New("unsupported resource in source")
)
