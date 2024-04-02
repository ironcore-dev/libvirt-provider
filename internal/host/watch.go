// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package host

import (
	"github.com/ironcore-dev/libvirt-provider/api"
	"github.com/ironcore-dev/libvirt-provider/internal/store"
)

type watch[E api.Object] struct {
	store  *Store[E]
	events chan store.WatchEvent[E]
}

func (w *watch[E]) Stop() {
	w.store.watchesMu.Lock()
	defer w.store.watchesMu.Unlock()

	w.store.watches.Delete(w)
}

func (w *watch[E]) Events() <-chan store.WatchEvent[E] {
	return w.events
}
