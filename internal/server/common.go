// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0
package server

import (
	"errors"

	"github.com/ironcore-dev/provider-utils/storeutils/store"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	ErrMachineNotFound            = errors.New("machine not found")
	ErrNicNotFound                = errors.New("nic not found")
	ErrVolumeNotFound             = errors.New("volume not found")
	ErrMachineIsntManaged         = errors.New("machine isn't managed")
	ErrMachineUnavailable         = errors.New("machine isn't available")
	ErrPCIControllerMaxedOut      = errors.New("pci controllers count already maxed out")
	ErrActiveConsoleSessionExists = errors.New("active console session exists for this domain")
	ErrInvalidRequest             = errors.New("invalid request")
)

func convertInternalErrorToGRPC(err error) error {
	_, ok := status.FromError(err)
	if ok {
		return err
	}

	code := codes.Internal

	switch {
	case errors.Is(err, store.ErrNotFound), errors.Is(err, ErrMachineNotFound), errors.Is(err, ErrVolumeNotFound), errors.Is(err, ErrNicNotFound):
		code = codes.NotFound
	case errors.Is(err, ErrMachineIsntManaged):
		code = codes.InvalidArgument
	case errors.Is(err, ErrPCIControllerMaxedOut):
		code = codes.ResourceExhausted
	case errors.Is(err, ErrActiveConsoleSessionExists):
		code = codes.FailedPrecondition
	case errors.Is(err, ErrMachineUnavailable):
		code = codes.Unavailable
	case errors.Is(err, ErrInvalidRequest):
		code = codes.InvalidArgument
	}

	return status.Error(code, err.Error())
}
