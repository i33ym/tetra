// Package blob provides an S3-compatible object store abstraction backed by
// MinIO. Payload bytes live here; metadata lives in Postgres.
package blob

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// putBufferThreshold bounds how much of an unknown-size upload is buffered in
// memory. Objects up to this size are written with a single exact-sized PUT
// (cheap); larger objects fall back to true multipart streaming. This avoids
// minio-go allocating ~16 MiB multipart part buffers for every small upload,
// which OOMs the process under concurrency.
const putBufferThreshold = 4 << 20 // 4 MiB

// Config defines the information needed to connect to the object store.
type Config struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Bucket    string
	UseSSL    bool
	Region    string
}

// Object is a readable object pulled from the store.
type Object struct {
	Body        io.ReadCloser
	Size        int64
	ContentType string
}

// Store is a handle to a single bucket in the object store.
type Store struct {
	client *minio.Client
	bucket string
	region string
}

// Open constructs a Store. It does not perform any network I/O.
func Open(cfg Config) (*Store, error) {
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("minio new: %w", err)
	}

	return &Store{
		client: client,
		bucket: cfg.Bucket,
		region: cfg.Region,
	}, nil
}

// Bucket returns the configured bucket name.
func (s *Store) Bucket() string {
	return s.bucket
}

// EnsureBucket creates the bucket if it does not already exist (idempotent).
func (s *Store) EnsureBucket(ctx context.Context) error {
	exists, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return fmt.Errorf("bucket exists: %w", err)
	}
	if exists {
		return nil
	}

	if err := s.client.MakeBucket(ctx, s.bucket, minio.MakeBucketOptions{Region: s.region}); err != nil {
		return fmt.Errorf("make bucket: %w", err)
	}

	return nil
}

// Put streams the reader to the object key. Pass size = -1 when the size is not
// known up front. It returns the authoritative number of bytes stored.
func (s *Store) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) (int64, error) {
	opts := minio.PutObjectOptions{ContentType: contentType}

	// Known size: hand off directly.
	if size >= 0 {
		info, err := s.client.PutObject(ctx, s.bucket, key, r, size, opts)
		if err != nil {
			return 0, fmt.Errorf("put object %q: %w", key, err)
		}
		return info.Size, nil
	}

	// Unknown size: read up to the threshold (+1 byte to detect overflow).
	head, err := io.ReadAll(io.LimitReader(r, putBufferThreshold+1))
	if err != nil {
		return 0, fmt.Errorf("buffer object %q: %w", key, err)
	}

	// Small object: single exact-sized PUT, no multipart buffers.
	if len(head) <= putBufferThreshold {
		opts.DisableMultipart = true
		info, err := s.client.PutObject(ctx, s.bucket, key, bytes.NewReader(head), int64(len(head)), opts)
		if err != nil {
			return 0, fmt.Errorf("put object %q: %w", key, err)
		}
		return info.Size, nil
	}

	// Large object: stream the buffered head plus the remainder via multipart.
	opts.PartSize = 16 << 20
	reader := io.MultiReader(bytes.NewReader(head), r)
	info, err := s.client.PutObject(ctx, s.bucket, key, reader, -1, opts)
	if err != nil {
		return 0, fmt.Errorf("put object %q: %w", key, err)
	}
	return info.Size, nil
}

// Get returns a streaming reader for the object at key. The caller must Close
// the returned Object's Body.
func (s *Store) Get(ctx context.Context, key string) (*Object, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("get object %q: %w", key, err)
	}

	info, err := obj.Stat()
	if err != nil {
		obj.Close()
		return nil, fmt.Errorf("stat object %q: %w", key, err)
	}

	return &Object{
		Body:        obj,
		Size:        info.Size,
		ContentType: info.ContentType,
	}, nil
}

// Remove deletes the object at key.
func (s *Store) Remove(ctx context.Context, key string) error {
	if err := s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("remove object %q: %w", key, err)
	}
	return nil
}

// HealthCheck verifies connectivity to the object store.
func (s *Store) HealthCheck(ctx context.Context) error {
	if _, err := s.client.BucketExists(ctx, s.bucket); err != nil {
		return fmt.Errorf("blob health: %w", err)
	}
	return nil
}
