# test libvirt-provider

[![REUSE status](https://api.reuse.software/badge/github.com/ironcore-dev/libvirt-provider)](https://api.reuse.software/info/github.com/ironcore-dev/libvirt-provider)
[![GitHub License](https://img.shields.io/static/v1?label=License&message=Apache-2.0&color=blue)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/ironcore-dev/libvirt-provider)](https://goreportcard.com/report/github.com/ironcore-dev/libvirt-provider)
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](https://makeapullrequest.com)

`libvirt-provider` is a Libvirt based provider implementation of the [ironcore](https://github.com/ironcore-dev/ironcore) `Machine` type.

Please consult the [project documentation](https://ironcore-dev.github.io/libvirt-provider/) for additional information.

### How it works
This project aims to follow the Kubernetes [Operator pattern](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/)

It uses [Controllers](https://kubernetes.io/docs/concepts/architecture/controller/)
which provides a reconcile function responsible for synchronizing resources until the desired state is reached on the cluster

## License

[Apache-2.0](LICENSE)

## Test ignore it
