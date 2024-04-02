// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package qcow2

import (
	"fmt"
	"sort"
)

type QCow2 interface {
	Create(filename string, opts ...CreateOption) error
}

type CreateOption interface {
	ApplyToCreate(o *CreateOptions)
}

type WithSize int64

func (s WithSize) ApplyToCreate(o *CreateOptions) {
	o.Size = (*int64)(&s)
}

type WithSourceFile string

func (s WithSourceFile) ApplyToCreate(o *CreateOptions) {
	o.SourceFile = string(s)
}

type CreateOptions struct {
	Size       *int64
	SourceFile string
}

func (o *CreateOptions) ApplyToCreate(o2 *CreateOptions) {
	if o.Size != nil {
		o2.Size = o.Size
	}
	if o.SourceFile != "" {
		o2.SourceFile = o.SourceFile
	}
}

func (o *CreateOptions) ApplyOptions(opts []CreateOption) {
	for _, opt := range opts {
		opt.ApplyToCreate(o)
	}
}

type qcow2AndPriority struct {
	qcow2    QCow2
	priority int
}

type implsRegistry struct {
	entries map[string]qcow2AndPriority
}

func (r *implsRegistry) Add(name string, priority int, qcow2 QCow2) error {
	if _, ok := r.entries[name]; ok {
		return fmt.Errorf("implementation %q already registered", name)
	}

	if r.entries == nil {
		r.entries = make(map[string]qcow2AndPriority)
	}

	r.entries[name] = qcow2AndPriority{qcow2, priority}
	return nil
}

func (r *implsRegistry) Instance(name string) (QCow2, error) {
	entry, ok := r.entries[name]
	if !ok {
		return nil, fmt.Errorf("unknown implementation %q", name)
	}
	return entry.qcow2, nil
}

func (r *implsRegistry) Available() []string {
	res := make([]string, 0, len(r.entries))
	for name := range r.entries {
		res = append(res, name)
	}
	sort.Slice(res, func(i, j int) bool {
		return r.entries[res[i]].priority < r.entries[res[j]].priority
	})
	return res
}

func (r *implsRegistry) Default() string {
	return r.Available()[0]
}

var (
	impls implsRegistry

	Instance  = impls.Instance
	Available = impls.Available
	Default   = impls.Default
)
