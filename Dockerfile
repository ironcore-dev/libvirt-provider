# Build the libvirt-provider binary
FROM --platform=$BUILDPLATFORM golang:1.23-bookworm AS builder

WORKDIR /workspace

# Copy the Go Modules manifests
COPY go.mod go.sum ./

# Cache dependencies before copying source code
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg \
    go mod download

# Copy the Go source code
COPY api/ api/
COPY internal/ internal/
COPY cmd/ cmd/
COPY hack/ hack/

ARG TARGETOS
ARG TARGETARCH
ARG BUILDPLATFORM
ENV BUILDARCH=amd64

# Install common dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    qemu-user-static qemu-utils ca-certificates \
    libvirt-clients libcephfs-dev librbd-dev librados-dev libc-bin \
    gcc g++ \
    && update-ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Install cross-compiler for ARM64 if building for arm64 on an amd64 host
RUN if [ "$TARGETARCH" = "arm64" ] && [ "$BUILDARCH" = "amd64" ]; then \
      dpkg --add-architecture arm64 && \
      apt-get update && apt-get install -y --no-install-recommends \
      gcc-aarch64-linux-gnu librbd-dev:arm64 librados-dev:arm64 libc6-dev:arm64; \
    fi

# Install cross-compiler for AMD64 if building for amd64 on an arm64 host
RUN if [ "$TARGETARCH" = "amd64" ] && [ "$BUILDARCH" = "arm64" ]; then \
      apt-get install -y --no-install-recommends \
      gcc g++; \
    fi

# Set compiler and linker flags based on target architecture
ENV CC=""
ENV CGO_LDFLAGS=""

RUN if [ "$TARGETARCH" = "arm64" ]; then \
      export CC="/usr/bin/aarch64-linux-gnu-gcc"; \
      export CGO_LDFLAGS="-L/usr/lib/aarch64-linux-gnu -Wl,-lrados -Wl,-lrbd"; \
    elif [ "$TARGETARCH" = "amd64" ]; then \
      export CC="/usr/bin/gcc"; \
      export CGO_LDFLAGS="-L/usr/lib/x86_64-linux-gnu -Wl,-lrados -Wl,-lrbd"; \
    fi && \
    CGO_ENABLED=1 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    CC="$CC" CGO_LDFLAGS="$CGO_LDFLAGS" \
    go build -ldflags="-s -w -linkmode=external" -o libvirt-provider ./cmd/libvirt-provider/main.go

# Install irictl-machine
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH GO111MODULE=on \
    go install github.com/ironcore-dev/ironcore/irictl-machine/cmd/irictl-machine@main

# Ensure the binary is in a common location
RUN if [ "$TARGETARCH" = "amd64" ]; then \
        mv /go/bin/irictl-machine /workspace/irictl-machine; \
    else \
        mv /go/bin/linux_$TARGETARCH/irictl-machine /workspace/irictl-machine; \
    fi

# Since we're leveraging apt to pull in dependencies, we use `gcr.io/distroless/base` because it includes glibc.
FROM gcr.io/distroless/base-debian11 AS distroless-base

# The distroless amd64 image has a target triplet of x86_64
FROM distroless-base AS distroless-amd64
ENV LIB_DIR_PREFIX=x86_64
ENV LIB_DIR_PREFIX_MINUS=x86-64
ENV LIB_DIR_SUFFIX_NUMBER=2
ENV LIB_DIR=lib64

# The distroless arm64 image has a target triplet of aarch64
FROM distroless-base AS distroless-arm64
ENV LIB_DIR_PREFIX=aarch64
ENV LIB_DIR_PREFIX_MINUS=aarch64
ENV LIB_DIR_SUFFIX_NUMBER=1
ENV LIB_DIR=lib

FROM busybox:1.37.0-uclibc AS busybox
FROM distroless-$TARGETARCH  AS libvirt-provider
WORKDIR /
COPY --from=busybox /bin/sh /bin/sh
COPY --from=busybox /bin/mkdir /bin/mkdir
COPY --from=builder /lib/${LIB_DIR_PREFIX}-linux-gnu/librados.so.2 \
/lib/${LIB_DIR_PREFIX}-linux-gnu/librbd.so.1 \
/lib/${LIB_DIR_PREFIX}-linux-gnu/libc.so.6 \
/lib/${LIB_DIR_PREFIX}-linux-gnu/libfmt.so.9 \
/lib/${LIB_DIR_PREFIX}-linux-gnu/libstdc++.so.6 \
/lib/${LIB_DIR_PREFIX}-linux-gnu/libgcc_s.so.1 \
/lib/${LIB_DIR_PREFIX}-linux-gnu/libssl.so.3 \
/lib/${LIB_DIR_PREFIX}-linux-gnu/libcryptsetup.so.12 \
/lib/${LIB_DIR_PREFIX}-linux-gnu/libcrypto.so.3 \
/lib/${LIB_DIR_PREFIX}-linux-gnu/libresolv.so.2 \
/lib/${LIB_DIR_PREFIX}-linux-gnu/libboost_thread.so.1.74.0 \
/lib/${LIB_DIR_PREFIX}-linux-gnu/libboost_iostreams.so.1.74.0 \
/lib/${LIB_DIR_PREFIX}-linux-gnu/libblkid.so.1 \
/lib/${LIB_DIR_PREFIX}-linux-gnu/libudev.so.1 \
/lib/${LIB_DIR_PREFIX}-linux-gnu/libibverbs.so.1 \
/lib/${LIB_DIR_PREFIX}-linux-gnu/librdmacm.so.1 \
/lib/${LIB_DIR_PREFIX}-linux-gnu/libm.so.6 \
/lib/${LIB_DIR_PREFIX}-linux-gnu/libuuid.so.1 \
/lib/${LIB_DIR_PREFIX}-linux-gnu/libdevmapper.so.1.02.1 \
/lib/${LIB_DIR_PREFIX}-linux-gnu/libargon2.so.1 \
/lib/${LIB_DIR_PREFIX}-linux-gnu/libjson-c.so.5 \
/lib/${LIB_DIR_PREFIX}-linux-gnu/libz.so.1 \
/lib/${LIB_DIR_PREFIX}-linux-gnu/libbz2.so.1.0 \
/lib/${LIB_DIR_PREFIX}-linux-gnu/liblzma.so.5 \
/lib/${LIB_DIR_PREFIX}-linux-gnu/libzstd.so.1 \
/lib/${LIB_DIR_PREFIX}-linux-gnu/libnl-route-3.so.200 \
/lib/${LIB_DIR_PREFIX}-linux-gnu/libnl-3.so.200 \
/lib/${LIB_DIR_PREFIX}-linux-gnu/libselinux.so.1 \
/lib/${LIB_DIR_PREFIX}-linux-gnu/libpthread.so.0 \
/lib/${LIB_DIR_PREFIX}-linux-gnu/libpcre2-8.so.0 /lib/${LIB_DIR_PREFIX}-linux-gnu/
RUN mkdir -p /${LIB_DIR}
COPY --from=builder /${LIB_DIR}/ld-linux-${LIB_DIR_PREFIX_MINUS}.so.${LIB_DIR_SUFFIX_NUMBER} /${LIB_DIR}/
RUN mkdir -p /usr/lib/${LIB_DIR_PREFIX}-linux-gnu/ceph/
COPY --from=builder /usr/lib/${LIB_DIR_PREFIX}-linux-gnu/ceph/libceph-common.so.2 /usr/lib/${LIB_DIR_PREFIX}-linux-gnu/ceph
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

COPY --from=builder /workspace/libvirt-provider /libvirt-provider
COPY --from=builder /workspace/irictl-machine /irictl-machine

USER 65532:65532

ENTRYPOINT ["/libvirt-provider"]
