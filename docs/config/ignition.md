# Ignition

The following is a minimal Ignition configuration that contains everything libvirt-provider-specific to get the service up and running.

```yaml
variant: fcos
version: 1.3.0
systemd:
  units:
  - name: postinstall.service
    enabled: true
    contents: |
      [Unit]
      Description=Post configuration service
      ConditionFirstBoot=yes
      After=systemd-networkd-wait-online.service inventory.service
      Wants=inventory.service

      [Service]
      Type=oneshot
      ExecStart=/opt/postinstall.sh

      [Install]
      WantedBy=multi-user.target
storage:
  directories:
  - path: /var/lib/libvirt-provider
    user:
      name: libvirt-provider
    group:
      name: libvirt-provider
    mode: 0755
  files:
  - path: /opt/postinstall.sh
    mode: 0755
    overwrite: yes
    contents:
      inline: |
        #!/usr/bin/env bash
        set -Eeuo pipefail
        export DEBIAN_FRONTEND=noninteractive
  
        apt-get update
        apt-get install -y libvirt-daemon-system libvirt-clients ovmf ceph-common
  
        # libvirt-qemu user runs the VM processes,
        # so it also needs access to the libvirt-provider group to read/write the disk images
        usermod -aG libvirt-provider libvirt-qemu
  
        # libvirt-provider user needs to be in the libvirt group to interact with the
        # libvirt daemon socket
        usermod -aG libvirt libvirt-provider
passwd:
  groups:
    - name: libvirt-provider
      gid: 65532 # specific GID used in the IronCore context
  users:
    - name: libvirt-provider
      uid: 65532 # specific UID used in the IronCore context
      primary_group: libvirt-provider
      home_dir: "/nonexistent" # Ubuntu/Debian standard for system users without home directories
      no_create_home: true
      no_user_group: true
      shell: "/sbin/nologin"
```
