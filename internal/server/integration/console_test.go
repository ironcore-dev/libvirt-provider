// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package integration_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/digitalocean/go-libvirt"
	iri "github.com/ironcore-dev/ironcore/iri/apis/machine/v1alpha1"
	irimeta "github.com/ironcore-dev/ironcore/iri/apis/meta/v1alpha1"
	libvirtutils "github.com/ironcore-dev/libvirt-provider/internal/libvirt/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/tools/remotecommand"
	transportspdy "k8s.io/client-go/transport/spdy"
	"k8s.io/streaming/pkg/httpstream/spdy"
)

// These specs validate the fix for
// https://github.com/ironcore-dev/libvirt-provider/issues/788: a machine
// console must be opened through libvirtd (virDomainOpenConsole) and carry
// data in both directions, instead of opening the guest console's PTY by
// host path. The former works regardless of libvirt's per-VM mount
// namespacing; the latter breaks as soon as the console PTY lives in QEMU's
// private devpts only.
//
// Each spec boots a real GardenLinux guest, waits for its getty login prompt
// on the serial console (proving the console output direction), then writes
// a unique marker to the console's stdin and waits for the guest's tty layer
// to echo it back. The echoed marker can only appear if the whole path —
// client, libvirt-provider, libvirtd, QEMU serial device, guest console ->
// and back — carries data in both directions.
var _ = Describe("Bidirectional console", func() {
	const (
		bootTimeout  = 5 * time.Minute
		echoTimeout  = 30 * time.Second
		closeTimeout = 30 * time.Second
		specTimeout  = 15 * time.Minute

		echoMarker = "ironcore-exec-echo-probe"
	)

	// validIgnitionData is a minimal but valid Ignition config. The
	// GardenLinux image boots with ignition.firstboot=1 and can fall into
	// emergency mode - with no serial login prompt - if the supplied config
	// does not parse.
	validIgnitionData := []byte(`{"ignition":{"version":"3.3.0"}}`)

	createBootedMachine := func(ctx SpecContext) (string, libvirt.Domain) {
		By("creating a machine with the GardenLinux boot image")
		createResp, err := machineClient.CreateMachine(ctx, &iri.CreateMachineRequest{
			Machine: &iri.Machine{
				Metadata: &irimeta.ObjectMetadata{},
				Spec: &iri.MachineSpec{
					Power:        iri.Power_POWER_ON,
					Class:        machineClassx3xlarge,
					IgnitionData: validIgnitionData,
					Volumes: []*iri.Volume{
						{
							Name:   "rootdisk",
							Device: "oda",
							LocalDisk: &iri.LocalDisk{
								Image: &iri.ImageSpec{Image: osImage},
							},
						},
					},
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(createResp).NotTo(BeNil())
		machineID := createResp.Machine.Metadata.Id

		By("waiting for the domain to exist and be running")
		var domain libvirt.Domain
		Eventually(func(g Gomega) libvirt.DomainState {
			d, err := libvirtConn.DomainLookupByUUID(libvirtutils.UUIDStringToBytes(machineID))
			g.Expect(err).NotTo(HaveOccurred())
			domain = d
			domainState, _, err := libvirtConn.DomainGetState(domain, 0)
			g.Expect(err).NotTo(HaveOccurred())
			return libvirt.DomainState(domainState)
		}).WithTimeout(bootTimeout).WithPolling(5 * time.Second).Should(Equal(libvirt.DomainRunning))

		return machineID, domain
	}

	It("should open a bidirectional console stream through libvirtd", SpecTimeout(specTimeout), func(ctx SpecContext) {
		machineID, domain := createBootedMachine(ctx)
		DeferCleanup(cleanupMachine(machineID))

		By("opening the domain console through libvirtd, like `virsh console` does")
		stdinR, stdinW := io.Pipe()
		sess := newConsoleSession(func(stdout io.Writer) error {
			return libvirtConn.DomainOpenConsoleBidirectionalIroncore(domain, nil, stdinR, stdout, 0)
		})

		By("waiting for the guest's login prompt (console output direction)")
		expectConsoleOutput(sess, "login:", bootTimeout)

		By("writing a marker to the console input and expecting the guest to echo it (console input direction)")
		_, err := stdinW.Write([]byte(echoMarker + "\n"))
		Expect(err).NotTo(HaveOccurred())
		expectConsoleOutput(sess, echoMarker, echoTimeout)

		By("closing stdin, which must cleanly end the console stream")
		Expect(stdinW.Close()).To(Succeed())
		expectConsoleClosed(sess, closeTimeout)
	})

	It("should stream a bidirectional console session through the machine exec url", SpecTimeout(specTimeout), func(ctx SpecContext) {
		machineID, _ := createBootedMachine(ctx)
		DeferCleanup(cleanupMachine(machineID))

		By("getting an exec url for the machine")
		execResp, err := machineClient.Exec(ctx, &iri.ExecRequest{MachineId: machineID})
		Expect(err).NotTo(HaveOccurred())
		execURL, err := url.ParseRequestURI(execResp.Url)
		Expect(err).NotTo(HaveOccurred())

		By("starting the SPDY exec stream, like `kubectl ironcore exec` does")
		stdinR, stdinW := io.Pipe()
		sess := newConsoleSession(func(stdout io.Writer) error {
			return streamExecConsole(ctx, execURL, stdinR, stdout)
		})

		By("ensuring the escape hint is framed for raw terminals (CRLF, not bare LF)")
		expectConsoleOutput(sess, "Escape character is ^] (Ctrl + ])\r\n", echoTimeout)

		By("waiting for the guest's login prompt (console output direction)")
		expectConsoleOutput(sess, "login:", bootTimeout)

		By("writing a marker to the exec stdin and expecting the guest to echo it (console input direction)")
		_, err = stdinW.Write([]byte(echoMarker + "\n"))
		Expect(err).NotTo(HaveOccurred())
		expectConsoleOutput(sess, echoMarker, echoTimeout)

		By("sending the Ctrl-] escape character, which must close the exec session without error")
		_, err = stdinW.Write([]byte{0x1d}) // Ctrl-] - the documented escape sequence
		Expect(err).NotTo(HaveOccurred())
		expectConsoleClosed(sess, closeTimeout)
	})
})

// streamExecConsole opens an SPDY exec stream against the given exec url and
// copies between the caller-provided streams until the session ends. It is
// runExec with live, caller-controlled stdio instead of canned buffers.
func streamExecConsole(ctx context.Context, execURL *url.URL, stdin io.Reader, stdout io.Writer) error {
	roundTripper, err := spdy.NewRoundTripperWithConfig(spdy.RoundTripperConfig{
		TLS:        http.DefaultTransport.(*http.Transport).TLSClientConfig,
		Proxier:    http.ProxyFromEnvironment,
		PingPeriod: 5 * time.Second,
	})
	if err != nil {
		return err
	}
	exec, err := remotecommand.NewSPDYExecutorForTransports(roundTripper, transportspdy.NewUpgraderForStreaming(roundTripper), http.MethodGet, execURL)
	if err != nil {
		return err
	}
	return exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdin:  stdin,
		Stdout: stdout,
		Tty:    true,
	})
}

// lockedBuffer guards a bytes.Buffer shared between the goroutine streaming
// console output and the test's polling assertions.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// consoleSession is a running console stream. err is assigned before done is
// closed, so reading it after done is closed is race-free.
type consoleSession struct {
	out  *lockedBuffer
	done chan struct{}
	err  error
}

func newConsoleSession(run func(stdout io.Writer) error) *consoleSession {
	s := &consoleSession{out: &lockedBuffer{}, done: make(chan struct{})}
	go func() {
		defer GinkgoRecover()
		defer close(s.done)
		s.err = run(s.out)
	}()
	return s
}

// expectConsoleOutput waits until the console session produced substr. If
// the session ends first, the stream error surfaces in the failure message
// instead of a bare "substring not found" timeout.
func expectConsoleOutput(s *consoleSession, substr string, timeout time.Duration) {
	Eventually(func(g Gomega) {
		select {
		case <-s.done:
			g.Expect(s.err).NotTo(HaveOccurred())
		default:
		}
		g.Expect(s.out.String()).To(ContainSubstring(substr))
	}).WithTimeout(timeout).WithPolling(500 * time.Millisecond).Should(Succeed())
}

// expectConsoleClosed waits for the console session to end without error.
func expectConsoleClosed(s *consoleSession, timeout time.Duration) {
	Eventually(s.done).WithTimeout(timeout).Should(BeClosed())
	Expect(s.err).NotTo(HaveOccurred())
}
