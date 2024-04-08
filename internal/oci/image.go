// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package oci

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/containerd/containerd/remotes"
	"github.com/go-logr/logr"
	ironcoreimage "github.com/ironcore-dev/ironcore-image"
	"github.com/ironcore-dev/ironcore-image/oci/image"
	"github.com/ironcore-dev/ironcore-image/oci/indexer"
	"github.com/ironcore-dev/ironcore-image/oci/remote"
	"github.com/ironcore-dev/ironcore-image/oci/store"
	"github.com/ironcore-dev/ironcore-image/utils/sets"
	ocispecv1 "github.com/opencontainers/image-spec/specs-go/v1"
)

type Image struct {
	Config    ironcoreimage.Config
	RootFS    *FileLayer
	InitRAMFs *FileLayer
	Kernel    *FileLayer
}

type FileLayer struct {
	Descriptor ocispecv1.Descriptor
	Path       string
}

type LocalCache struct {
	mu      sync.Mutex
	running bool

	log logr.Logger

	store    *store.Store
	registry *remote.Registry

	pullRequests chan pullRequest
	listeners    []Listener
}

type pullRequest struct {
	ctx context.Context
	ref string
	res chan pullResult
}

type pullResult struct {
	image *Image
	err   error
}

func readImageConfig(ctx context.Context, img image.Image) (*ironcoreimage.Config, error) {
	configLayer, err := img.Config(ctx)
	if err != nil {
		return nil, fmt.Errorf("error getting config layer: %w", err)
	}

	rc, err := configLayer.Content(ctx)
	if err != nil {
		return nil, fmt.Errorf("error getting config content: %w", err)
	}
	defer func() { _ = rc.Close() }()

	config := &ironcoreimage.Config{}
	if err := json.NewDecoder(rc).Decode(config); err != nil {
		return nil, fmt.Errorf("error decoding config: %w", err)
	}
	return config, nil
}

func (c *LocalCache) resolveImage(ctx context.Context, ociImg image.Image) (*Image, error) {
	config, err := readImageConfig(ctx, ociImg)
	if err != nil {
		return nil, err
	}

	layers, err := ociImg.Layers(ctx)
	if err != nil {
		return nil, fmt.Errorf("error getting oci layers: %w", err)
	}

	var (
		localStore = c.store.Layout().Store()
		img        = Image{Config: *config}
	)
	for _, layer := range layers {
		switch layer.Descriptor().MediaType {
		case ironcoreimage.InitRAMFSLayerMediaType:
			initRAMFSPath, err := localStore.BlobPath(layer.Descriptor().Digest)
			if err != nil {
				return nil, fmt.Errorf("error getting path to initramfs: %w", err)
			}
			img.InitRAMFs = &FileLayer{
				Descriptor: layer.Descriptor(),
				Path:       initRAMFSPath,
			}
		case ironcoreimage.KernelLayerMediaType:
			kernelPath, err := localStore.BlobPath(layer.Descriptor().Digest)
			if err != nil {
				return nil, fmt.Errorf("error getting path to kernel: %w", err)
			}
			img.Kernel = &FileLayer{
				Descriptor: layer.Descriptor(),
				Path:       kernelPath,
			}
		case ironcoreimage.RootFSLayerMediaType:
			rootFSPath, err := localStore.BlobPath(layer.Descriptor().Digest)
			if err != nil {
				return nil, fmt.Errorf("error getting path to rootfs: %w", err)
			}
			img.RootFS = &FileLayer{
				Descriptor: layer.Descriptor(),
				Path:       rootFSPath,
			}

		}
	}
	var missing []string
	if img.RootFS == nil || img.RootFS.Path == "" {
		missing = append(missing, "rootfs")
	}
	if img.Kernel == nil || img.Kernel.Path == "" {
		missing = append(missing, "kernel")
	}
	if img.InitRAMFs == nil || img.InitRAMFs.Path == "" {
		missing = append(missing, "initramfs")
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("incomplete oci: components are missing: %v", missing)
	}

	return &img, nil
}

func (c *LocalCache) loop(ctx context.Context) {
	var (
		activePulls = sets.New[string]()
		pullDone    = make(chan string)
	)

	for {
		select {
		case <-ctx.Done():
			return
		case ref := <-pullDone:
			activePulls.Delete(ref)
			for _, listener := range c.listeners {
				listener.HandlePullDone(PullDoneEvent{Ref: ref})
			}
		case req := <-c.pullRequests:
			req.ctx = setupMediaTypeKeyPrefixes(ctx)
			if activePulls.Has(req.ref) {
				req.res <- pullResult{err: ErrImagePulling}
				continue
			}

			ociImg, err := c.store.Resolve(req.ctx, req.ref)
			if err != nil {
				if !errors.Is(err, indexer.ErrNotFound) {
					req.res <- pullResult{err: fmt.Errorf("error pulling %s: %w", req.ref, err)}
				}

				activePulls.Insert(req.ref)
				go func() {
					log := c.log.WithValues("Ref", req.ref)
					defer func() {
						select {
						case pullDone <- req.ref:
						case <-ctx.Done():
						}
					}()

					log.V(1).Info("Start pulling")
					err := c.retryPullImage(ctx, req.ref)
					if err != nil {
						log.Error(err, "Error copying oci")
						return
					}
					log.V(1).Info("Successfully pulled")
				}()

				req.res <- pullResult{err: ErrImagePulling}
				continue
			}

			img, err := c.resolveImage(req.ctx, ociImg)
			req.res <- pullResult{image: img, err: err}
		}
	}
}

func (c *LocalCache) retryPullImage(ctx context.Context, ref string) error {
	var maxRetries = 5
	var errs []error
	log := c.log.WithValues("Ref", ref).V(1)

	sleepDuration := 1 * time.Second
	for i := 0; i < maxRetries; i++ {
		if i > 0 {
			sleepDuration <<= 2
			select {
			case <-time.After(sleepDuration):
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		err := c.pullImage(ctx, ref)
		if err == nil {
			return nil
		}
		log.Error(err, "oci couldn't be pulled")
		errs = append(errs, fmt.Errorf("trial %d of oci pull failed with: %w ", i+1, err))
	}

	return fmt.Errorf("exceeded max retries, oci pull failed with error(s): %v", errs)
}

func (c *LocalCache) pullImage(ctx context.Context, ref string) error {
	sourceImg, err := image.Copy(ctx, c.store, c.registry, ref)
	if err != nil {
		return err
	}
	ociImg, err := c.store.Resolve(ctx, ref)
	if err != nil {
		return fmt.Errorf("error resolving ref locally %s: %w", ref, err)
	}

	srcDigest := sourceImg.Descriptor().Digest
	copiedDigest := ociImg.Descriptor().Digest

	if srcDigest != copiedDigest {
		if err = c.store.Delete(ctx, ref); err != nil {
			return fmt.Errorf("error deleting oci from local oci store %s: %w", ref, err)
		}
		return fmt.Errorf("oci digest verification failed for oci %s: source digest %s, copied digest %s", ref, srcDigest, copiedDigest)
	}
	return nil
}

var ErrImagePulling = errors.New("oci pulling")

func IgnoreImagePulling(err error) error {
	if errors.Is(err, ErrImagePulling) {
		return nil
	}
	return err
}

func setupMediaTypeKeyPrefixes(ctx context.Context) context.Context {
	mediaTypeToPrefix := map[string]string{
		ironcoreimage.ConfigMediaType:         "config",
		ironcoreimage.InitRAMFSLayerMediaType: "layer",
		ironcoreimage.KernelLayerMediaType:    "layer",
		ironcoreimage.RootFSLayerMediaType:    "layer",
	}
	for mediaType, prefix := range mediaTypeToPrefix {
		ctx = remotes.WithMediaTypeKeyPrefix(ctx, mediaType, prefix)
	}
	return ctx
}

func (c *LocalCache) Start(ctx context.Context) error {
	ctx = setupMediaTypeKeyPrefixes(ctx)

	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return fmt.Errorf("already running")
	}

	c.running = true
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.running = false
	}()
	c.loop(ctx)

	return nil
}

func NewLocalCache(log logr.Logger, registry *remote.Registry, store *store.Store) (*LocalCache, error) {
	return &LocalCache{
		log:          log,
		store:        store,
		registry:     registry,
		pullRequests: make(chan pullRequest),
	}, nil
}

type Cache interface {
	Get(ctx context.Context, ref string) (*Image, error)
	AddListener(listener Listener)
}

type PullDoneEvent struct {
	Ref string
}

type Listener interface {
	HandlePullDone(evt PullDoneEvent)
}

type ListenerFuncs struct {
	HandlePullDoneFunc func(evt PullDoneEvent)
}

func (l ListenerFuncs) HandlePullDone(evt PullDoneEvent) {
	if l.HandlePullDoneFunc != nil {
		l.HandlePullDoneFunc(evt)
	}
}

func (c *LocalCache) Get(ctx context.Context, ref string) (*Image, error) {
	c.mu.Lock()
	running := c.running
	c.mu.Unlock()

	if !running {
		return nil, fmt.Errorf("need to start manager first")
	}

	pullRes := make(chan pullResult, 1)

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case c.pullRequests <- pullRequest{ctx, ref, pullRes}:
	}

	// Once successfully submitted, it's expected to get a result that respects the given
	// context.Context, hence we're just reading instead of doing a `select` with the context.Context of this
	// function.
	res := <-pullRes
	return res.image, res.err
}

func (c *LocalCache) AddListener(listener Listener) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.listeners = append(c.listeners, listener)
}
