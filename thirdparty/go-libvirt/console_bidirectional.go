// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0
//
// IronCore patch: bidirectional console streaming.
//
// Upstream now exports DomainOpenConsoleBidirectional (added 2026-06-09 via
// generator special-casing), but its requestStream orchestration deadlocks on
// stream teardown when both directions are used: processIncomingStream is
// called twice and the second call blocks on getResponse after the stream
// already ended; the abort channel is additionally unbuffered, which can hang
// the error path once the sender goroutine has exited. See
// https://github.com/digitalocean/go-libvirt/issues/260 (still open).
//
// libvirt-provider needs both directions and clean teardown to back
// `kubectl ironcore exec`
// (https://github.com/ironcore-dev/libvirt-provider/issues/788), so this file
// provides an IronCore variant that avoids requestStream. Drop this file, the
// replace directive, and the vendored copy in favor of the upstream API once
// go-libvirt fixes bidirectional stream teardown.

package libvirt

import (
	"io"

	"github.com/digitalocean/go-libvirt/internal/constants"
	"github.com/digitalocean/go-libvirt/socket"
)

// procDomainOpenConsole is REMOTE_PROC_DOMAIN_OPEN_CONSOLE. The generated
// DomainOpenConsole passes the same literal to requestStream; we need it here
// because the client->server stream packets must carry it.
const procDomainOpenConsole = 201

// DomainOpenConsoleBidirectionalIroncore opens a bidirectional stream to a
// domain console, equivalent to the C libvirt virDomainOpenConsole
// bidirectional mode (and to what `virsh console` does). Console output
// (server->client) is written to stdout until the stream ends; console input
// (client->server) is read from stdin and streamed to libvirtd.
//
// An empty devName (nil) selects the first console. flags is currently unused
// upstream for input; pass 0.
//
// Unlike upstream DomainOpenConsoleBidirectional, teardown does not deadlock.
// The stream ends when:
//   - stdin reaches EOF: a stream-finish (OK) is sent and libvirtd closes the
//     console, mirroring Ctrl-] in virsh console. The caller should translate
//     its escape sequence into an EOF on stdin.
//   - libvirtd ends the console stream (e.g. domain shutdown or a forced
//     console takeover elsewhere).
func (l *Libvirt) DomainOpenConsoleBidirectionalIroncore(dom Domain, devName OptString, stdin io.Reader, stdout io.Writer, flags uint32) error {
	args := DomainOpenConsoleArgs{
		Dom:     dom,
		DevName: devName,
		Flags:   flags,
	}

	payload, err := encode(&args)
	if err != nil {
		return err
	}

	serial := l.serial()
	c := make(chan response)

	l.register(serial, c)
	defer func() {
		l.cmux.Lock()
		defer l.cmux.Unlock()
		l.deregister(serial)
	}()

	err = l.socket.SendPacket(serial, procDomainOpenConsole, constants.Program, payload, socket.Call, socket.StatusOK)
	if err != nil {
		return err
	}

	// Wait for the console-open acknowledgement.
	if _, err = l.getResponse(c); err != nil {
		return err
	}

	// Client->server half: stream stdin into the console.
	//
	// abort must be buffered: SendStream checks it only between stdin reads,
	// so by the time the incoming half ends the sender may already have
	// returned (e.g. stdin hit EOF first) and an unbuffered abort send would
	// block forever.
	abort := make(chan bool, 1)
	outErr := make(chan error, 1)
	go func() {
		outErr <- l.socket.SendStream(serial, procDomainOpenConsole, constants.Program, stdin, abort)
	}()

	// Server->client half: stream console output into stdout until libvirtd
	// ends the stream. This is the call that blocks for the session.
	_, inErr := l.processIncomingStream(c, stdout)

	// The session is over. Signal the sender to stop if it is between reads;
	// if it is blocked reading stdin it exits once the caller closes stdin as
	// the session tears down. Never wait for it here: that would deadlock
	// when libvirtd ended the stream while stdin was idle.
	select {
	case abort <- true:
	default:
	}

	// Surface a sender error only if one has already been reported; do not
	// block on it.
	if inErr == nil {
		select {
		case err := <-outErr:
			inErr = err
		default:
		}
	}

	return inErr
}
