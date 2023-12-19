// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
	"github.com/digitalocean/go-libvirt"
	"github.com/go-logr/logr"
	iri "github.com/ironcore-dev/ironcore/iri/apis/machine/v1alpha1"
	remotecommandserver "github.com/ironcore-dev/ironcore/poollet/machinepoollet/iri/streaming/remotecommand"
	"github.com/ironcore-dev/libvirt-provider/pkg/api"
	libvirtutils "github.com/ironcore-dev/libvirt-provider/pkg/libvirt/utils"
	"github.com/moby/term"
	"k8s.io/client-go/tools/remotecommand"
)

const (
	StreamCreationTimeout = 30 * time.Second
	StreamIdleTimeout     = 2 * time.Minute
)

type executorExec struct {
	Libvirt         *libvirt.Libvirt
	ExecRequest     *iri.ExecRequest
	VirshExecutable string
	Machine         *api.Machine
}

func (s *Server) Exec(ctx context.Context, req *iri.ExecRequest) (*iri.ExecResponse, error) {
	machineID := req.MachineId
	log := s.loggerFrom(ctx, "MachineID", machineID)

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
		return
	}

	exec := executorExec{
		Libvirt:         s.libvirt,
		ExecRequest:     request,
		VirshExecutable: s.virshExecutable,
		Machine:         apiMachine,
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
	var wg sync.WaitGroup

	machineID := e.ExecRequest.MachineId
	log := logr.FromContextOrDiscard(ctx).WithName(machineID)

	// Check if the apiMachine doesn't exist, to avoid making the libvirt-lookup call.
	if e.Machine == nil {
		return fmt.Errorf("apiMachine not found")
	}

	_, err := e.Libvirt.DomainLookupByUUID(libvirtutils.UUIDStringToBytes(machineID))
	if err != nil {
		if !libvirtutils.IsErrorCode(err, libvirt.ErrNoDomain) {
			return fmt.Errorf("error looking up domain: %w", err)
		}

		return fmt.Errorf("machine %s has not yet been synced", machineID)
	}

	uri, err := e.Libvirt.ConnectGetUri()
	if err != nil {
		return fmt.Errorf("error getting connection uri")
	}

	cmd := exec.CommandContext(ctx, e.VirshExecutable, "-c", strings.TrimSpace(uri), "console", machineID)

	f, err := pty.Start(cmd)
	if err != nil {
		return fmt.Errorf("error starting command: %w", err)
	}

	// Wrap the input stream with an escape proxy. Escape Sequence Ctrl + ] = 29
	inputReader := term.NewEscapeProxy(in, []byte{29})

	wg.Add(2)
	// ReadInput: go routine to read the input from the reader, and write to the terminal.
	go func() {
		defer wg.Done()

		buf := make([]byte, 1024)
		for {
			n, err := inputReader.Read(buf)
			if err != nil {
				if _, ok := err.(term.EscapeError); ok {
					_, _ = f.Write(buf) // This is to close the writer.
					log.Info("Closed reading the terminal. Escape sequence received")
					return
				}
				log.Error(err, "error reading bytes")
				return
			}

			_, err = f.Write(buf[:n])
			if err != nil {
				log.Error(err, "error writing to the file descriptor")
				return
			}
		}
	}()

	// WriteOutput: go routine for writing the output back to the Writer.
	go func() {
		defer wg.Done()
		// Ignoring error to allow graceful shutdown without flagging as an error; not needed at this stage.
		_, _ = io.Copy(out, f)
		log.Info("Closed writing to the terminal")
	}()

	if err := cmd.Wait(); err != nil {
		// Avoid returning so that the function can verify if all go routines are terminated.
		log.Error(err, "console command interrupted")
	}

	wg.Wait()
	log.Info("Closed console for the machine")
	return nil
}
