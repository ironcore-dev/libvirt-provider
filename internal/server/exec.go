// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/digitalocean/go-libvirt"
	"github.com/go-logr/logr"
	iri "github.com/ironcore-dev/ironcore/iri/apis/machine/v1alpha1"
	remotecommandserver "github.com/ironcore-dev/ironcore/poollet/machinepoollet/iri/streaming/remotecommand"
	"github.com/ironcore-dev/libvirt-provider/api"
	libvirtutils "github.com/ironcore-dev/libvirt-provider/internal/libvirt/utils"
	"github.com/ironcore-dev/provider-utils/storeutils/store"
	"k8s.io/client-go/tools/remotecommand"
)

const (
	StreamCreationTimeout = 30 * time.Second
	StreamIdleTimeout     = 2 * time.Minute
)

// consoleEscapeByte is the escape byte terminating an exec console session:
// Ctrl-] (0x1d), the same escape character `virsh console` uses.
const consoleEscapeByte = 0x1d

// consoleEscapeReader truncates the console input at the consoleEscapeByte
// and reports io.EOF from then on, so the console stream terminates cleanly
// when the user presses the escape character. Data between escape bytes - as
// happens with large pastes and cursor movement - is passed through
// untouched; only input up to the first escape byte is forwarded.
type consoleEscapeReader struct {
	r io.Reader
	// done latches once the escape byte or the end of the underlying reader
	// has been reached; from then on Read reports io.EOF.
	done bool
}

func newConsoleEscapeReader(r io.Reader) io.Reader {
	return &consoleEscapeReader{r: r}
}

func (r *consoleEscapeReader) Read(p []byte) (int, error) {
	if r.done {
		return 0, io.EOF
	}
	n, err := r.r.Read(p)
	if i := bytes.IndexByte(p[:n], consoleEscapeByte); i >= 0 {
		r.done = true
		if i == 0 {
			return 0, io.EOF
		}
		return i, nil
	}
	if err == io.EOF {
		r.done = true
		if n > 0 {
			// Deliver the data first; report io.EOF on the next call.
			return n, nil
		}
		return 0, io.EOF
	}
	return n, err
}

type executorExec struct {
	Libvirt        *libvirt.Libvirt
	ExecRequest    *iri.ExecRequest
	Machine        *api.Machine
	activeConsoles *sync.Map
}

func (s *Server) Exec(ctx context.Context, req *iri.ExecRequest) (*iri.ExecResponse, error) {
	log := s.loggerFrom(ctx, "MachineID", req.MachineId)
	log.V(1).Info("Verifying machine in the store")
	if _, err := s.machineStore.Get(ctx, req.MachineId); err != nil {
		return nil, convertInternalErrorToGRPC(fmt.Errorf("error getting machine: %w", err))
	}

	log.V(1).Info("Inserting request into cache")
	token, err := s.execRequestCache.Insert(req)
	if err != nil {
		return nil, err
	}

	log.V(1).Info("Returning url with token")
	return &iri.ExecResponse{
		Url: s.buildURL("exec", token),
	}, nil
}

func (s *Server) ServeExec(w http.ResponseWriter, req *http.Request, token string) {
	ctx := req.Context()
	log := logr.FromContextOrDiscard(ctx)

	request, ok := s.execRequestCache.Consume(token)
	if !ok {
		log.V(1).Info("Rejecting unknown / expired token")
		http.NotFound(w, req)
		return
	}
	apiMachine, err := s.machineStore.Get(ctx, request.MachineId)
	if err != nil {
		log.Error(err, "error getting the apiMachine")
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, req)
			return
		}
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	exec := executorExec{
		Libvirt:        s.libvirt,
		ExecRequest:    request,
		Machine:        apiMachine,
		activeConsoles: &s.activeConsoles,
	}

	handler, err := remotecommandserver.NewExecHandler(exec, remotecommandserver.ExecHandlerOptions{
		StreamCreationTimeout: StreamCreationTimeout,
		StreamIdleTimeout:     StreamIdleTimeout,
	})
	if err != nil {
		log.Error(err, "error creating exec handler")
		code := http.StatusInternalServerError
		http.Error(w, http.StatusText(code), code)
		return
	}

	handler.Handle(w, req, remotecommandserver.ExecOptions{})
}

func (e executorExec) Exec(ctx context.Context, in io.Reader, out io.WriteCloser, _ remotecommand.TerminalSizeQueue) error {
	machineID := e.ExecRequest.MachineId

	// Check if a console is already active for this machine
	_, loaded := e.activeConsoles.LoadOrStore(machineID, true)
	if loaded {
		return convertInternalErrorToGRPC(fmt.Errorf("operation failed: %w", ErrActiveConsoleSessionExists))
	}

	defer e.activeConsoles.Delete(machineID)

	// Check if the apiMachine doesn't exist, to avoid making the libvirt-lookup call.
	if e.Machine == nil {
		return convertInternalErrorToGRPC(fmt.Errorf("apiMachine %w in the store", ErrMachineNotFound))
	}

	domain, err := e.Libvirt.DomainLookupByUUID(libvirtutils.UUIDStringToBytes(machineID))
	if err != nil {
		if !libvirtutils.IsErrorCode(err, libvirt.ErrNoDomain) {
			return convertInternalErrorToGRPC(fmt.Errorf("error looking up domain: %w", err))
		}

		return convertInternalErrorToGRPC(fmt.Errorf("machine %s has not yet been synced: %w %w", machineID, ErrMachineUnavailable, err))
	}

	log := logr.FromContextOrDiscard(ctx).WithName(machineID)
	log.Info("Opening bidirectional console stream through libvirtd")

	// Wrap the input stream with the escape reader, truncating the input at the
	// escape character (Ctrl + ]), which cleanly ends the console stream.
	inputReader := newConsoleEscapeReader(in)

	fmt.Fprintf(out, "Escape character is ^] (Ctrl + ])\n")

	// Open the machine console through libvirtd (like `virsh console`) and
	// stream it bidirectionally. Opening the console PTY by its host path
	// does not work when libvirt runs the QEMU process in a private mount
	// namespace (the libvirt default), where the PTY only exists in the
	// guest's private devpts.
	if err := e.Libvirt.DomainOpenConsoleBidirectionalIroncore(domain, nil, inputReader, out, 0); err != nil {
		return convertInternalErrorToGRPC(fmt.Errorf("error streaming console: %w", err))
	}

	log.Info("Closed console for the machine")
	return nil
}
