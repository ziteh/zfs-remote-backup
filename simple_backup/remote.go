package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// RemoteBackend defines the interface for remote storage
type RemoteBackend interface {
	Upload(ctx context.Context, localPath, remotePath string, sha256Hash string) error
}

// S3Backend implements RemoteBackend for AWS S3
type S3Backend struct {
	client         *s3.Client
	uploader       *manager.Uploader
	bucket         string
	prefix         string
	storageClass   types.StorageClass
	customEndpoint bool
}

// NewS3Backend creates a new S3 backend
func NewS3Backend(ctx context.Context, bucket, region, prefix, endpoint, storageClass string) (*S3Backend, error) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	var client *s3.Client
	if endpoint != "" {
		client = s3.NewFromConfig(cfg, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(endpoint)
			o.UsePathStyle = true
		})
		log.Printf("S3 client initialized with custom endpoint: %s", endpoint)
	} else {
		// AWS S3
		client = s3.NewFromConfig(cfg)
	}

	uploader := manager.NewUploader(client)

	// Parse storage class
	var sc types.StorageClass
	if storageClass != "" {
		sc = types.StorageClass(storageClass)
		log.Printf("Using storage class: %s", storageClass)
	} else {
		return nil, fmt.Errorf("storage class must be specified")
	}

	return &S3Backend{
		client:         client,
		uploader:       uploader,
		bucket:         bucket,
		prefix:         prefix,
		storageClass:   sc,
		customEndpoint: endpoint != "",
	}, nil
}

// Upload uploads a file to S3 with SHA256 checksum for integrity verification
func (s *S3Backend) Upload(ctx context.Context, localPath, remotePath string, sha256Hash string) error {
	file, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	key := filepath.ToSlash(filepath.Join(s.prefix, remotePath))

	input := &s3.PutObjectInput{
		Bucket:       aws.String(s.bucket),
		Key:          aws.String(key),
		Body:         file,
		StorageClass: s.storageClass,
	}
	// Include checksum only if not using custom endpoint
	if !s.customEndpoint && sha256Hash != "" {
		input.ChecksumAlgorithm = types.ChecksumAlgorithmSha256
		input.ChecksumSHA256 = aws.String(sha256Hash)
	}

	_, err = s.uploader.Upload(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to upload to S3: %w", err)
	}

	log.Printf("  Uploaded to S3: s3://%s/%s (storage class: %s)", s.bucket, key, s.storageClass)
	return nil
}
