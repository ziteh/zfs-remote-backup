package remote

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type ObjectInfo struct {
	Size   int64
	Blake3 string
}

type Backend interface {
	Upload(ctx context.Context, localPath, remotePath, checksumHash string, backupLevel int16) error
	Head(ctx context.Context, remotePath string) (*ObjectInfo, error)
	VerifyCredentials(ctx context.Context) error
}

type S3 struct {
	client         *s3.Client
	uploader       *manager.Uploader
	bucket         string
	prefix         string
	storageClass   types.StorageClass
	customEndpoint bool
}

func NewS3(ctx context.Context, bucket, region, prefix, endpoint string, storageClass types.StorageClass, maxRetryAttempts int) (*S3, error) {
	var configOpts []func(*awsconfig.LoadOptions) error
	configOpts = append(configOpts, awsconfig.WithRegion(region))

	if maxRetryAttempts > 0 {
		configOpts = append(configOpts,
			awsconfig.WithRetryMaxAttempts(maxRetryAttempts),
			awsconfig.WithRetryMode(aws.RetryModeStandard),
		)
		slog.Info("Configured S3 retry strategy", "mode", "standard", "maxAttempts", maxRetryAttempts)
	}

	cfg, err := awsconfig.LoadDefaultConfig(ctx, configOpts...)
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
		client = s3.NewFromConfig(cfg)
	}

	uploader := manager.NewUploader(client, func(u *manager.Uploader) {
		u.PartSize = 64 * 1024 * 1024
		u.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenSupported
	})

	if storageClass == "" {
		return nil, fmt.Errorf("storage class must be specified")
	}
	slog.Info("Using storage class", "storageClass", storageClass)

	return &S3{
		client:         client,
		uploader:       uploader,
		bucket:         bucket,
		prefix:         prefix,
		storageClass:   storageClass,
		customEndpoint: endpoint != "",
	}, nil
}

func (s *S3) Download(ctx context.Context, remotePath, localPath string) error {
	key := filepath.ToSlash(filepath.Join(s.prefix, remotePath))

	file, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("failed to create local file: %w", err)
	}
	defer file.Close()

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

func (s *S3) Upload(ctx context.Context, localPath, remotePath, checksumHash string, backupLevel int16) error {
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

	input := &s3.PutObjectInput{
		Bucket:       aws.String(s.bucket),
		Key:          aws.String(key),
		Body:         file,
		StorageClass: s.storageClass,
		Tagging:      aws.String("backup-level=" + levelTag),
		Metadata:     map[string]string{"blake3": checksumHash},
	}

	_, err = s.uploader.Upload(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to upload to S3: %w", err)
	}

	slog.Info("Uploaded to S3", "bucket", s.bucket, "key", key, "storageClass", s.storageClass)
	return nil
}

func (s *S3) Head(ctx context.Context, remotePath string) (*ObjectInfo, error) {
	key := filepath.ToSlash(filepath.Join(s.prefix, remotePath))

	output, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to head object %s: %w", key, err)
	}

	info := &ObjectInfo{}
	if output.ContentLength != nil {
		info.Size = *output.ContentLength
	}
	if output.Metadata != nil {
		info.Blake3 = output.Metadata["blake3"]
	}
	return info, nil
}

func (s *S3) VerifyCredentials(ctx context.Context) error {
	slog.Info("Verifying AWS credentials and bucket access", "bucket", s.bucket)

	_, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(s.bucket),
	})
	if err != nil {
		return fmt.Errorf("failed to verify AWS credentials or bucket access: %w", err)
	}

	slog.Info("AWS credentials verified successfully", "bucket", s.bucket)
	return nil
}

func ValidateStorageClass(storageClass string) error {
	if storageClass == "GLACIER" || storageClass == "DEEP_ARCHIVE" {
		return fmt.Errorf("storage class %s is not immediately accessible (requires restore)", storageClass)
	}
	return nil
}
