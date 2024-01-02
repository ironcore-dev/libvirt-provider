// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"fmt"
	"os"
	"syscall"

	"github.com/ironcore-dev/libvirt-provider/provider/cmd/app"
	originzap "go.uber.org/zap"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	errTerminatedApp = "app terminated with error"
)

func main() {
	ctx := ctrl.SetupSignalHandler()

	cmd, ptrLogger := app.Command()
	exitCode := int(0)
	if err := cmd.ExecuteContext(ctx); err != nil {
		exitCode = 1

		// logger can be nil, if flags weren't parsed
		if *ptrLogger != nil {
			(*ptrLogger).Error(errTerminatedApp, originzap.NamedError("error", err))
		} else {
			fmt.Fprintln(os.Stderr, errTerminatedApp+": "+err.Error())
		}
	}

	if *ptrLogger != nil {
		err := (*ptrLogger).Sync()
		// https://github.com/uber-go/zap/issues/991
		if err != nil && !(errors.Is(err, syscall.ENOTTY) || errors.Is(err, syscall.EINVAL)) {
			// it cannot be printed with logger, this error will extremely rare hopefully
			fmt.Fprintln(os.Stderr, "flushing of logger buffer wasn't successfully:", err)
		}
	}

	os.Exit(exitCode)

}
