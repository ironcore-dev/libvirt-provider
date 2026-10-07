// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"bytes"
	"io"
	"testing"

	. "github.com/onsi/gomega"
)

// TestConsoleEscapeReader verifies that console input up to the escape
// character (Ctrl-], 0x1d) passes through and that the escape produces a
// clean end-of-input signalled via io.EOF. A clean end (rather than an
// error) is what makes the libvirt console stream close gracefully.
func TestConsoleEscapeReader(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		wantRead []byte
	}{
		{
			name:     "plain input passes through unchanged",
			input:    []byte("ls -la\n"),
			wantRead: []byte("ls -la\n"),
		},
		{
			name:     "escape character truncates input and drops the rest",
			input:    []byte{'a', 0x1d, 'b'},
			wantRead: []byte{'a'},
		},
		{
			name:     "escape character as first byte yields empty input",
			input:    []byte{0x1d},
			wantRead: []byte{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			r := newConsoleEscapeReader(bytes.NewReader(tt.input))

			got, err := io.ReadAll(r)
			g.Expect(err).NotTo(HaveOccurred(), "escape must surface as clean end-of-input (io.EOF)")
			g.Expect(got).To(Equal(tt.wantRead))
		})
	}
}

// TestConsoleEscapeReaderStreaming covers the interactive paste pattern over
// a live pipe. moby/term's NewEscapeProxy, used by the previous
// implementation, deadlocks here, because it holds a trailing escape byte
// and tries to read ahead to classify it, blocking on an idle pipe instead
// of delivering the already received data.
func TestConsoleEscapeReaderStreaming(t *testing.T) {
	readNext := func(r io.Reader) (string, error) {
		t.Helper()
		buf := make([]byte, 1024)
		n, err := r.Read(buf)
		return string(buf[:n]), err
	}

	// io.Pipe is synchronous: writes block until a read consumes them, so the
	// writer runs in its own goroutine and closes the pipe when done.
	writeAndClose := func(pw *io.PipeWriter, data []byte) {
		go func() {
			defer pw.Close()
			_, _ = pw.Write(data)
		}()
	}

	// A chunk with a trailing escape byte, as produced when pasting a command
	// followed by the escape character, must immediately deliver the data
	// before the escape byte.
	t.Run("immediately delivers data of a chunk with a trailing escape byte", func(t *testing.T) {
		g := NewWithT(t)

		pr, pw := io.Pipe()
		r := newConsoleEscapeReader(pr)
		writeAndClose(pw, []byte("hello\x1d"))

		got, err := readNext(r)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(got).To(Equal("hello"))

		got, err = readNext(r)
		g.Expect(err).To(MatchError(io.EOF))
		g.Expect(got).To(BeEmpty())
	})

	// EOF accompanying data must deliver the data first and report EOF only
	// on the next call.
	t.Run("delivers data accompanying EOF first and reports EOF only on the next call", func(t *testing.T) {
		g := NewWithT(t)

		pr, pw := io.Pipe()
		r := newConsoleEscapeReader(pr)
		writeAndClose(pw, []byte("tail"))

		got, err := readNext(r)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(got).To(Equal("tail"))

		got, err = readNext(r)
		g.Expect(err).To(MatchError(io.EOF))
		g.Expect(got).To(BeEmpty())
	})
}
