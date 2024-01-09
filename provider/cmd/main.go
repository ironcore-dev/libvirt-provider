// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/ironcore-dev/libvirt-provider/provider/cmd/app"
	ctrl "sigs.k8s.io/controller-runtime"
)

const ()

func main() {
	ctx := ctrl.SetupSignalHandler()

	if err := app.Command().ExecuteContext(ctx); err != nil && !errors.Is(err, app.ErrIgnore) {
		fmt.Fprintln(os.Stderr, app.MsgErrTerminatedApp+": "+err.Error())
		os.Exit(1)
	}
}
