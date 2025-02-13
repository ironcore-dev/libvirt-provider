# Libvirt-provider - usage documentation

## Overview

`libvirt-provider` enables interaction with `libvirt` for managing virtual machine instances, integrating with multiple plugins for networking, storage, and more. It provides a flexible architecture to handle resources like VMs, volumes, and networking interfaces, with built-in support for garbage collection, volume size resyncing, and health monitoring.
This guide provides a comprehensive usage flow, including configuration, flags, and practical examples, to get your `libvirt-provider` instance running.

---

## Prerequisites

Before you begin, make sure you have the prerequisites described [here](development/dev_setup.md#prerequisites).

---

## Building from source

To build `libvirt-provider` from the source:

1. **Clone the repository**

    To bring up and start locally the libvirt-provider project, you first need to clone the repository.

    ```bash
    git clone git@github.com:ironcore-dev/libvirt-provider.git
    cd libvirt-provider
    ```

1. **Build the `libvirt-provider`**

    ```bash
    make build
    ```

---

## Configuration

`libvirt-provider` is configured via command-line flags. To see the full list of configuration options, run:

```bash
libvirt-provider -h
```

### Required flags

The following flags are required for the application to run properly:

`--supported-machine-classes` (Path to the supported machine classes file). Sample `machine-classes.json` can be found [here](../config/development/machineclasses.json).

---

## Running libvirt-provider

Below is an example of configuring and running `libvirt-provider` with various flags:

```bash
go run cmd/libvirt-provider/main.go \
  --address /var/run/iri-machinebroker.sock \
  --streaming-address ":20251" \
  --base-url "http://localhost:20251" \
  --libvirt-socket /var/run/libvirt/libvirt-sock \
  --libvirt-address "unix:///var/run/libvirt/libvirt-sock" \
  --libvirt-uri "qemu:///system" \
  --enable-hugepages \
  --qcow2-type "qcow2" \
  --volume-cache-policy "writeback" \
  --servers-metrics-address ":9090" \
  --servers-health-check-address ":8080" \
  --gc-vm-graceful-shutdown-timeout 5m \
  --gc-resync-interval 1m \
  --supported-machine-classes "/home/libvirt-provider/machine-classes.json"
```

Once the libvirt-provider is started, which will handle various tasks like connecting to libvirt, managing VMs, and exposing various HTTP and gRPC endpoints for metrics, health checks, and more.

## Server endpoints

1. **gRPC endpoint**: Exposes various gRPC services to manage virtual machines.
Default address: `unix:///var/run/iri-machinebroker.sock`

1. **Metrics server**: Provides Prometheus-compatible metrics for monitoring.
Default address: `""` (if configured)

1. **Health check server**: Provides a simple liveness check endpoint.
Default address: `:8181` (if configured)

1. **Streaming server**: Streams VM status and events.
Default address: `:20251` (if configured)

You can access these services by connecting to their respective addresses.

---

## Logs and debugging

The service logs are critical for troubleshooting issues. By default, the service uses the `zap` logging framework, which supports structured logging and multiple log levels.

To change the logging level:

```bash
go run cmd/libvirt-provider/main.go --zap-log-level debug
```

For more advanced troubleshooting, you can enable additional logging at different points in the execution flow.

---

## Troubleshooting

Here are some common issues you might encounter:

### Libvirt connection errors

If the `libvirt` socket or URI is incorrectly configured, the service will fail to connect to the hypervisor. Ensure that:

1. The `libvirt` socket path is correct.

1. The `libvirt` URI is accessible (e.g., `qemu:///system` or `unix:///var/run/libvirt/libvirt-sock`).

### VM launch failures

If VMs fail to launch, check the logs for specific errors related to machine classes or network plugins. You may need to adjust the supported machine classes or verify your network plugin configuration.
