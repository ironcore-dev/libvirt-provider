# Local Development Setup

- [Prerequisites](#prerequisites)
- [Preperation](#preperation)
- [Run libvirt-provider for local development](#run-libvirt-provider-for-local-development)
- [Interact with the `libvirt-provider`](#interact-with-the-libvirt-provider)
- [Deploy `libvirt-provider`](#deploy-libvirt-provider)

> ℹ️ **NOTE**:</br>
> To be able to take exec console of the machine, you can follow any one of the below approaches:</br>
> - Run the `libvirt-provider` as the `libvirt-qemu` user.</br>
> - Create an entry with `devpts /dev/pts devpts rw,nosuid,noexec,relatime,gid=5,mode=0660 0 0` in `/etc/fstab`.</br>
> - Manually ensure that you have `0660` access permissions on the character files created in `/dev/pts`.</br>

## Prerequisites

- Linux (code contains OS specific code)
- go >= 1.20
- `git`, `make` and `kubectl`
- Access to a Kubernetes cluster ([Minikube](https://minikube.sigs.k8s.io/docs/), [kind](https://kind.sigs.k8s.io/) or a
  real cluster)
- [libvirt](http://libvirt.org)
- [QEMU](https://www.qemu.org/download/)
- `irictl-machine` should be running locally or as [container](https://github.com/ironcore-dev/ironcore/pkgs/container/ironcore-irictl-machine)

## Preperation

### Setup `irictl-machine`

1. **Clone ironcore repository**

    ```bash
    git clone git@github.com:ironcore-dev/ironcore.git
    cd ironcore
    ```

2. **Build `irictl-machine`**

    ```bash
    go build -o bin/irictl-machine ./irictl-machine/cmd/irictl-machine/main.go
    ```

## Run libvirt-provider for local development

1. **Clone the Repository**

    To bring up and start locally the libvirt-provider project for development purposes you first need to clone the repository.

    ```bash
    git clone git@github.com:ironcore-dev/libvirt-provider.git
    cd libvirt-provider
    ```

1. **Build the `libvirt-provider`**

    ```bash
    make build
    ```

1. **Run the `libvirt-provider`**
   
    The required libvirt-provider flags needs to be defined:

    ```bash
    go run provider/cmd/main.go \
      --libvirt-provider-dir=<path-to-initialize-libvirt-provider> \
      --supported-machine-classes=<path-to-machine-class-json>/machine-classes.json \
      --network-interface-plugin-name=isolated \
      --address=<local-path>/iri-machinebroker.sock
    ```

    Sample `machine-classes.json` can be found [here](../../config/development/machineclasses.json).

## Interact with the `libvirt-provider`

1. **Creating machine**

    ```bash
    irictl-machine --address=unix:<local-path-to-socket>/iri-machinebroker.sock create machine -f <path-to-machine-yaml>/iri-machine.yaml
    ```

    Sample `iri-machine.yaml`:

    ```yaml
        metadata:
          id: 91076287116041d00fd421f43c3760389041dac4a8bd9201afba9a5baeb21c7
          labels:
            downward-api.machinepoollet.api.onmetal.de/root-machine-name: machine-hd4
            downward-api.machinepoollet.api.onmetal.de/root-machine-namespace: default
            downward-api.machinepoollet.api.onmetal.de/root-machine-uid: cab82eac-09d8-4428-9e6c-c98b40027b74
            machinepoollet.api.onmetal.de/machine-name: machine-hd4
            machinepoollet.api.onmetal.de/machine-namespace: default
            machinepoollet.api.onmetal.de/machine-uid: cab82eac-09d8-4428-9e6c-c98b40027b74
        spec:
          class: x3-small
          image:
            image: ghcr.io/ironcore-dev/ironcore-image/gardenlinux:rootfs-dev-20231206-v1
          volumes:
          - empty_disk:
              size_bytes: 5368709120
            name: ephe-disk
            device: oda
    ```

2. **Listing machines**

    ```bash
    irictl-machine --address=unix:<local-path-to-socket>/iri-machinebroker.sock get machine
    ```

3. **Deleting machine**

    ```bash
    irictl-machine --address=unix:<local-path-to-socket>/iri-machinebroker.sock delete machine <machine UUID>
    ```

4. **Taking machine console**

    ```bash
    irictl-machine --address=unix:<local-path-to-socket>/iri-machinebroker.sock exec <machine UUID>
    ```

## Deploy `libvirt-provider`

> ℹ️ **NOTE**:</br>
> If the `libvirt-uri` can not be auto-detected it can be defined via flag: e.g. `--libvirt-uri=qemu:///session`</br>
> ℹ️ **NOTE**:</br>
> For trying out the controller use the `isolated` network interface plugin: `--network-interface-plugin-name=isolated`</br>
> ℹ️ **NOTE**:</br>
> Libvirt-provider can run directly as binary program on worker node

1. **Make docker images**

    ```bash
    make docker-build
    ```

2. **Deploy virtlet as kubernetes**

    ```bash
    make deploy
    ```
    
