# Build the libvirt-provider binary
FROM --platform=$BUILDPLATFORM golang:1.24-bookworm AS builder

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
ENV BUILDARCH=${BUILDPLATFORM##*/}

# Install common dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    qemu-user-static qemu-utils ca-certificates \
    libvirt-clients libcephfs-dev librbd-dev librados-dev libc-bin \
    gcc g++ \
    && update-ca-certificates \
    && rm -rf /var/lib/apt/lists/*


RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH GO111MODULE=on go build -ldflags="-s -w" -a -o libvirt-provider ./cmd/libvirt-provider/main.go


# Install irictl-machine
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH GO111MODULE=on \
    go install github.com/ironcore-dev/ironcore/irictl-machine/cmd/irictl-machine@main

# Ensure the binary is in a common location
RUN if [ "$TARGETARCH" = "$BUILDARCH" ]; then \
        mv /go/bin/irictl-machine /workspace/irictl-machine; \
    else \
        mv /go/bin/linux_$TARGETARCH/irictl-machine /workspace/irictl-machine; \
    fi




COPY --from=builder /workspace/libvirt-provider /libvirt-provider
COPY --from=builder /workspace/irictl-machine /irictl-machine

USER 65532:65532

ENTRYPOINT ["/libvirt-provider"]
