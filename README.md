# libvirt-provider

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

Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
