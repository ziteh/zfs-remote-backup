package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// RemoteBackend defines the interface for remote storage
type RemoteBackend interface {
	Upload(ctx context.Context, localPath, remotePath, checksumHash string, backupLevel int16) error
	VerifyCredentials(ctx context.Context) error
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
func NewS3Backend(ctx context.Context, bucket, region, prefix, endpoint string, storageClass types.StorageClass, maxRetryAttempts int) (*S3Backend, error) {
	var configOpts []func(*config.LoadOptions) error
	configOpts = append(configOpts, config.WithRegion(region))

	// Apply retry configuration if specified
	if maxRetryAttempts > 0 {
		configOpts = append(configOpts,
			config.WithRetryMaxAttempts(maxRetryAttempts),
			config.WithRetryMode(aws.RetryModeStandard),
		)
		slog.Info("Configured S3 retry strategy", "mode", "standard", "maxAttempts", maxRetryAttempts)
	}

	cfg, err := config.LoadDefaultConfig(ctx, configOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	if endpoint != "" {
		if accessKey := os.Getenv("AWS_ACCESS_KEY_ID"); accessKey != "" {
			if secretKey := os.Getenv("AWS_SECRET_ACCESS_KEY"); secretKey != "" {
				cfg.Credentials = credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")
			}
		}
	}

	var client *s3.Client
	if endpoint != "" {
		client = s3.NewFromConfig(cfg, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(endpoint)
			o.UsePathStyle = true
		})
		slog.Info("S3 client initialized with custom endpoint", "endpoint", endpoint)
	} else {
		// AWS S3
		client = s3.NewFromConfig(cfg)
	}

	uploader := manager.NewUploader(client, func(u *manager.Uploader) {
		// In bytes
		u.PartSize = 64 * 1024 * 1024
		// Checksum is always calculated if supported. (Default)
		u.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenSupported
	})

	// Validate storage class
	if storageClass == "" {
		return nil, fmt.Errorf("storage class must be specified")
	}
	sc := storageClass
	slog.Info("Using storage class", "storageClass", sc)

	return &S3Backend{
		client:         client,
		uploader:       uploader,
		bucket:         bucket,
		prefix:         prefix,
		storageClass:   sc,
		customEndpoint: endpoint != "",
	}, nil
}

// Download downloads a file from S3 to local path
func (s *S3Backend) Download(ctx context.Context, remotePath, localPath string) error {
	key := filepath.ToSlash(filepath.Join(s.prefix, remotePath))

	// Create output file
	file, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("failed to create local file: %w", err)
	}
	defer file.Close()

	// Download from S3
	downloader := manager.NewDownloader(s.client)
	numBytes, err := downloader.Download(ctx, file, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("failed to download from S3: %w", err)
	}

	slog.Info("Downloaded from S3", "bucket", s.bucket, "key", key, "bytes", numBytes)
	return nil
}

func (s *S3Backend) Upload(ctx context.Context, localPath, remotePath, checksumHash string, backupLevel int16) error {
	var levelTag string
	if backupLevel < 0 {
		levelTag = "manifest"
	} else {
		levelTag = fmt.Sprint(backupLevel)
	}

	file, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	key := filepath.ToSlash(filepath.Join(s.prefix, remotePath))

	// Use manager.Uploader which automatically:
	// - Handles multipart uploads for files > PartSize
	// - Uploads parts concurrently (5 goroutines by default)
	// - Calculates CRC32 checksums automatically
	input := &s3.PutObjectInput{
		Bucket:       aws.String(s.bucket),
		Key:          aws.String(key),
		Body:         file,
		StorageClass: s.storageClass,
		Tagging:      aws.String("backup-level=" + levelTag),
	}

	_, err = s.uploader.Upload(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to upload to S3: %w", err)
	}

	slog.Info("Uploaded to S3", "bucket", s.bucket, "key", key, "storageClass", s.storageClass)
	return nil
}

// VerifyCredentials verifies that AWS credentials are valid and bucket is accessible
func (s *S3Backend) VerifyCredentials(ctx context.Context) error {
	slog.Info("Verifying AWS credentials and bucket access", "bucket", s.bucket)

	// Try to head the bucket to verify credentials and bucket access
	_, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(s.bucket),
	})
	if err != nil {
		return fmt.Errorf("failed to verify AWS credentials or bucket access: %w", err)
	}

	slog.Info("AWS credentials verified successfully", "bucket", s.bucket)
	return nil
}
