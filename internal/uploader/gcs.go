package uploader

import (
	"context"
	"fmt"
	"os"
	"time"
	"io"

	"cloud.google.com/go/storage"
)

type GCSUploader struct {
	Client *storage.Client
}

func NewGCSUploader(ctx context.Context) (*GCSUploader, error) {
	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, err
	}
	return &GCSUploader{Client: client}, nil
}

func (u *GCSUploader) UploadAndSign(ctx context.Context, bucket, object, filePath string, expiration time.Duration) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer f.Close()

	bkt := u.Client.Bucket(bucket)
	obj := bkt.Object(object)
	
	wc := obj.NewWriter(ctx)
	wc.ContentType = "text/html"
	if _, err = io.Copy(wc, f); err != nil {
		wc.Close()
		return "", fmt.Errorf("failed to write to gcs: %w", err)
	}
	if err := wc.Close(); err != nil {
		return "", fmt.Errorf("failed to close gcs writer: %w", err)
	}

	// Signed URL
	// Note: This requires credentials with Service Account Key (json) or similar.
	// If running in ADC that doesn't support signing (like GCE instance metadata), this might fail.
	
	opts := &storage.SignedURLOptions{
		Method:  "GET",
		Expires: time.Now().Add(expiration),
	}
	
	url, err := bkt.SignedURL(object, opts)
	if err != nil {
		// Fallback suggestion: if public access? 
		// But user asked for presigned.
		return "", fmt.Errorf("failed to sign url: %w", err)
	}

	return url, nil
}
