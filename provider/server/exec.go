// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/digitalocean/go-libvirt"
	"github.com/go-logr/logr"
	iri "github.com/ironcore-dev/ironcore/iri/apis/machine/v1alpha1"
	remotecommandserver "github.com/ironcore-dev/ironcore/poollet/machinepoollet/iri/streaming/remotecommand"
	"github.com/ironcore-dev/libvirt-provider/pkg/api"
	libvirtutils "github.com/ironcore-dev/libvirt-provider/pkg/libvirt/utils"
	"github.com/ironcore-dev/libvirt-provider/pkg/store"
	"github.com/moby/term"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/client-go/tools/remotecommand"
	"libvirt.org/go/libvirtxml"
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
	log := s.loggerFrom(ctx, "MachineID", req.MachineId)
	log.V(1).Info("Verifying machine in the store")
	if _, err := s.machineStore.Get(ctx, req.MachineId); err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			return nil, fmt.Errorf("error getting machine: %w", err)
		}
		return nil, status.Errorf(codes.NotFound, "machine %s not found", req.MachineId)
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
		return fmt.Errorf("apiMachine %w in the store", store.ErrNotFound)
	}

	domain, err := e.Libvirt.DomainLookupByUUID(libvirtutils.UUIDStringToBytes(machineID))
	if err != nil {
		if !libvirtutils.IsErrorCode(err, libvirt.ErrNoDomain) {
			return fmt.Errorf("error looking up domain: %w", err)
		}

		return fmt.Errorf("machine %s has not yet been synced", machineID)
	}

	domainXMLData, err := e.Libvirt.DomainGetXMLDesc(domain, 0)
	if err != nil {
		return fmt.Errorf("failed to lookup domain: %w", err)
	}

	domainXML := &libvirtxml.Domain{}
	if err := domainXML.Unmarshal(domainXMLData); err != nil {
		return fmt.Errorf("failed to unmarshal domain: %w", err)
	}

	ttyPath := domainXML.Devices.Consoles[0].TTY

	f, err := os.OpenFile(ttyPath, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("error opening PTY: %w", err)
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
					f.Close() // This is to close the writer.
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

	wg.Wait()
	log.Info("Closed console for the machine")
	return nil
}
