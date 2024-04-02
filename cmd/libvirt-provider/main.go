// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"

	"github.com/ironcore-dev/libvirt-provider/cmd/libvirt-provider/app"
	ctrl "sigs.k8s.io/controller-runtime"
)

func main() {
	ctx := ctrl.SetupSignalHandler()
	log := ctrl.Log.WithName("main")

	if err := app.Command().ExecuteContext(ctx); err != nil {
		log.V(1).Error(err, "error running libvirt provider")
		os.Exit(1)
	}
}
