package uploader

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type S3Uploader struct {
	Client *s3.Client
	PresignClient *s3.PresignClient
}

func NewS3Uploader(ctx context.Context, region string) (*S3Uploader, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	if region != "" {
		cfg.Region = region
	}

	client := s3.NewFromConfig(cfg)
	return &S3Uploader{
		Client: client,
		PresignClient: s3.NewPresignClient(client),
	}, nil
}

func (u *S3Uploader) UploadAndPresign(ctx context.Context, bucket, key, filePath string, expiration time.Duration) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Upload
	_, err = u.Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Body:   file,
		ContentType: aws.String("text/html"),
	})
	if err != nil {
		return "", fmt.Errorf("failed to upload to s3: %w", err)
	}

	// Presign
	req, err := u.PresignClient.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(expiration))
	if err != nil {
		return "", fmt.Errorf("failed to sign url: %w", err)
	}

	return req.URL, nil
}
