# Console

Every machine is equipped with a serial console which allows interactive access to the guest, e.g. via the
`Exec` endpoint of the IRI `MachineRuntime` service.

## Console log

The console output of a machine is additionally mirrored to a log file on the hypervisor. This is an always-on
feature and uses the libvirt [`<log>`](https://libvirt.org/formatdomain.html#element-log) sub-element of the
serial chardev, so the interactive console stays usable while all traffic is persisted.

The log file is stored per machine at:

```text
<libvirt-provider-dir>/machines/<machine-uid>/logs/console.log
```

It is written with `append="on"`, i.e. the content is preserved across domain restarts. This allows operators
to inspect early boot logs (firmware, bootloader, kernel) of a machine, which is especially useful when a
machine fails to boot.

The console log is removed together with the machine directory when the machine is deleted.
