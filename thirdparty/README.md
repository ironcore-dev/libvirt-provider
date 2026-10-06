# Vendored go-libvirt with bidirectional console support

`thirdparty/go-libvirt` is a reduced copy of the pinned upstream module
`github.com/digitalocean/go-libvirt v0.0.0-20260814190004-1a83157e1858`,
containing only the packages the provider builds against (root package,
`internal/constants`, `internal/event`, `internal/go-xdr/xdr2`, `socket`,
`socket/dialers`) plus one IronCore patch:

- `console_bidirectional.go` adds
  `(*Libvirt).DomainOpenConsoleBidirectionalIroncore`, a `virDomainOpenConsole`
  client that streams in **both** directions and tears the stream down
  cleanly.

The provider uses this to back `kubectl ironcore exec` by opening the guest
console through libvirtd instead of opening the console PTY by host path
(<https://github.com/ironcore-dev/libvirt-provider/issues/788>). Opening
through libvirtd works regardless of libvirt's per-VM mount namespacing
(`namespaces = [...]` in qemu.conf), under which the PTY path from the domain
XML only exists inside QEMU's private devpts and cannot be opened by the
provider container.

## Why not the upstream API?

Upstream added `DomainOpenConsoleBidirectional` on 2026-06-09 (generator
special-casing so `DomainOpenConsole` stays API-compatible). However, as of
`1a83157e1858` its implementation deadlocks on stream teardown when both
directions are used: `requestStream` calls `processIncomingStream` twice
(once in the `out != nil` branch, again in the trailing `switch in`), and the
second call blocks in `getResponse` after the stream already ended. The abort
channel is additionally unbuffered, so a sender goroutine that already exited
(e.g. stdin hit EOF first) would hang the error path. See the stale upstream
issue <https://github.com/digitalocean/go-libvirt/issues/260>.

The IronCore variant exists with a distinct name to avoid colliding with the
generated upstream method. It orchestrates `register`/`SendStream`/
`processIncomingStream` directly with a buffered abort channel and
non-blocking sender-error surfacing, which avoids both hazards. It was
validated end-to-end by the integration specs in
`internal/server/integration/console_test.go` (login prompt observed in
output; marker echoed for input; Ctrl-]/stdin-EOF tears the session down
cleanly).

Once upstream fixes bidirectional stream teardown, drop the patch file, the
`replace` directive in `go.mod`, and this directory, and switch the provider
to the upstream call.

## Licensing

The copy is covered by two entries in `REUSE.toml`: the go-libvirt sources are
Apache-2.0 by The go-libvirt Authors (their files carry license boilerplate
but no SPDX tags, so file-by-file detection fails), and
`internal/go-xdr` is davecgh/go-xdr under the ISC license (its `LICENSE` is
retained as required). `console_bidirectional.go` is IronCore's own patch and
carries its own in-file SPDX tag.

## Updating the copy

1. Extract the pinned upstream module (e.g. from the Go module cache) and copy
   only the needed subset into `thirdparty/go-libvirt/`, `chmod -R u+w` first
   (module-cache files are read-only):
   - root: `AUTHORS`, `LICENSE.md`, `README.md`, `go.mod`, `go.sum`, all
     non-test `.go` files;
   - `internal/constants`, `internal/event`, `socket` (incl. `dialers`) without
     test files;
   - `internal/go-xdr` keeping its `LICENSE` and `xdr2` non-test files.
   Everything else (`testdata`, `libvirttest`, `scripts`, `internal/lvgen`,
   `.github`, CI configs) is not needed to build the provider.
2. Re-apply `console_bidirectional.go` (it only depends on internals of the
   upstream package that have been stable across the bumps seen so far;
   adjust for upstream changes otherwise). Keep the `...Ironcore` method name
   as long as upstream's generated `DomainOpenConsoleBidirectional` exists,
   or the vendored copy will fail to compile with a redeclaration error.
3. Run the bidirectional console integration tests in
   `internal/server/integration` (label `integration`).
