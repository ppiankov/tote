package agent

import (
	"context"
	"fmt"
	"io"
	"sync"
)

// FakeImageStore implements ImageStore for testing.
type FakeImageStore struct {
	mu     sync.Mutex
	images map[string][]byte
	tags   map[string]string // imageRef -> digest
}

// NewFakeImageStore creates an empty fake image store.
func NewFakeImageStore() *FakeImageStore {
	return &FakeImageStore{
		images: make(map[string][]byte),
		tags:   make(map[string]string),
	}
}

// AddImage adds an image with the given digest and tar data.
func (f *FakeImageStore) AddImage(digest string, data []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.images[digest] = data
}

// AddTag maps an image reference to a digest.
func (f *FakeImageStore) AddTag(imageRef, digest string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tags[imageRef] = digest
}

// List returns all stored digests.
func (f *FakeImageStore) List(_ context.Context) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	digests := make([]string, 0, len(f.images))
	for d := range f.images {
		digests = append(digests, d)
	}
	return digests, nil
}

// Has returns true if the digest exists.
func (f *FakeImageStore) Has(_ context.Context, digest string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.images[digest]
	return ok, nil
}

// Size returns the byte length of the stored data for the digest.
func (f *FakeImageStore) Size(_ context.Context, digest string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	data, ok := f.images[digest]
	if !ok {
		return 0, fmt.Errorf("image %s not found", digest)
	}
	return int64(len(data)), nil
}

// ResolveTag returns the digest for an image reference if mapped.
func (f *FakeImageStore) ResolveTag(_ context.Context, imageRef string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	d, ok := f.tags[imageRef]
	if !ok {
		return "", nil
	}
	return d, nil
}

// Remove deletes an image by tag or digest.
func (f *FakeImageStore) Remove(_ context.Context, imageRef string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.tags[imageRef]; ok {
		delete(f.tags, imageRef)
		return nil
	}
	if _, ok := f.images[imageRef]; ok {
		delete(f.images, imageRef)
		return nil
	}
	return fmt.Errorf("image %s not found", imageRef)
}

// Export writes the stored tar data for the given digest.
func (f *FakeImageStore) Export(_ context.Context, digest string, w io.Writer) error {
	f.mu.Lock()
	data, ok := f.images[digest]
	f.mu.Unlock()
	if !ok {
		return fmt.Errorf("image %s not found", digest)
	}
	_, err := w.Write(data)
	return err
}

// Import reads tar data and stores it under a deterministic digest.
func (f *FakeImageStore) Import(_ context.Context, r io.Reader) (string, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}
	digest := fmt.Sprintf("sha256:fake-%d", len(data))
	f.mu.Lock()
	f.images[digest] = data
	f.mu.Unlock()
	return digest, nil
}

// FailingImageStore returns errors for all operations.
type FailingImageStore struct {
	Err error
}

func (f *FailingImageStore) List(_ context.Context) ([]string, error)               { return nil, f.Err }
func (f *FailingImageStore) Has(_ context.Context, _ string) (bool, error)          { return false, f.Err }
func (f *FailingImageStore) Size(_ context.Context, _ string) (int64, error)        { return 0, f.Err }
func (f *FailingImageStore) ResolveTag(_ context.Context, _ string) (string, error) { return "", f.Err }
func (f *FailingImageStore) Export(_ context.Context, _ string, _ io.Writer) error  { return f.Err }
func (f *FailingImageStore) Import(_ context.Context, _ io.Reader) (string, error)  { return "", f.Err }
func (f *FailingImageStore) Remove(_ context.Context, _ string) error               { return f.Err }
