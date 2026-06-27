package commands

import (
	"context"
	"fmt"
	"time"

	"github.com/i33ym/tetra/foundation/blob"
	"github.com/i33ym/tetra/foundation/config"
)

// MinioBootstrap creates the configured bucket if it does not already exist.
func MinioBootstrap(cfg config.MinIO) error {
	store, err := blob.Open(blob.Config{
		Endpoint:  cfg.Endpoint,
		AccessKey: cfg.AccessKey,
		SecretKey: cfg.SecretKey,
		Bucket:    cfg.Bucket,
		UseSSL:    cfg.UseSSL,
		Region:    cfg.Region,
	})
	if err != nil {
		return fmt.Errorf("open blob: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Retry so this works as a compose/k8s init step before MinIO is fully up.
	var lastErr error
	for attempt := 1; attempt <= 15; attempt++ {
		if err := store.EnsureBucket(ctx); err != nil {
			lastErr = err
			time.Sleep(2 * time.Second)
			continue
		}
		fmt.Printf("bucket %q ready\n", cfg.Bucket)
		return nil
	}

	return fmt.Errorf("ensure bucket after retries: %w", lastErr)
}
