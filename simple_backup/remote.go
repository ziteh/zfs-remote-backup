package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
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
	Upload(ctx context.Context, localPath, remotePath, sha256Hash string, backupLevel int16) error
}

// S3Backend implements RemoteBackend for AWS S3
type S3Backend struct {
	client         *s3.Client
	uploader       *manager.Uploader
	bucket         string
	prefix         string
	storageClass   types.StorageClass
	customEndpoint bool
	uploadStates   map[string]*multipartUploadState // key -> state
}

type multipartUploadState struct {
	UploadID  string
	Parts     []types.CompletedPart
	PartSize  int64
	TotalSize int64
	Uploaded  int64
}

// NewS3Backend creates a new S3 backend
func NewS3Backend(ctx context.Context, bucket, region, prefix, endpoint string, storageClass types.StorageClass) (*S3Backend, error) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
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
		uploadStates:   make(map[string]*multipartUploadState),
	}, nil
}

// Upload uploads a file to S3 with resumable multipart upload
func (s *S3Backend) Upload(ctx context.Context, localPath, remotePath, sha256Hash string, backupLevel int16) error {
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

	// Check if the object already exists on S3
	headOutput, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err == nil {
		localFileInfo, _ := os.Stat(localPath)
		if aws.ToInt64(headOutput.ContentLength) == localFileInfo.Size() {
			slog.Info("Object already exists on S3 with matching size, skipping upload", "bucket", s.bucket, "key", key)
			return nil
		}
	}

	// Get file size
	fileInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat file: %w", err)
	}
	fileSize := fileInfo.Size()

	// If file is small, use simple upload
	if fileSize < 100*1024*1024 { // 100MB
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

	// Multipart upload for large files
	return s.uploadMultipart(ctx, file, key, fileSize, levelTag)
}

// TODO: really need manual multipart upload? any high level library support?
func (s *S3Backend) uploadMultipart(ctx context.Context, file *os.File, key string, fileSize int64, levelTag string) error {
	partSize := int64(64 * 1024 * 1024) // 64MB
	stateKey := key

	// Check if there's an existing upload state
	state, exists := s.uploadStates[stateKey]
	if !exists {
		// Start new multipart upload
		createResp, err := s.client.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{
			Bucket:       aws.String(s.bucket),
			Key:          aws.String(key),
			StorageClass: s.storageClass,
			Tagging:      aws.String("backup-level=" + levelTag),
		})
		if err != nil {
			return fmt.Errorf("failed to create multipart upload: %w", err)
		}
		state = &multipartUploadState{
			UploadID:  *createResp.UploadId,
			Parts:     []types.CompletedPart{},
			PartSize:  partSize,
			TotalSize: fileSize,
			Uploaded:  0,
		}
		s.uploadStates[stateKey] = state
		slog.Info("Started multipart upload", "key", key, "uploadId", state.UploadID)
	}

	// Calculate number of parts
	numParts := (fileSize + partSize - 1) / partSize

	// Upload remaining parts
	for partNum := int32(len(state.Parts) + 1); partNum <= int32(numParts); partNum++ {
		offset := int64(partNum-1) * partSize
		size := partSize
		if offset+size > fileSize {
			size = fileSize - offset
		}

		buffer := make([]byte, size)
		_, err := file.ReadAt(buffer, offset)
		if err != nil && err != io.EOF {
			return fmt.Errorf("failed to read file part: %w", err)
		}

		uploadResp, err := s.client.UploadPart(ctx, &s3.UploadPartInput{
			Bucket:     aws.String(s.bucket),
			Key:        aws.String(key),
			PartNumber: aws.Int32(partNum),
			UploadId:   aws.String(state.UploadID),
			Body:       bytes.NewReader(buffer),
		})
		if err != nil {
			return fmt.Errorf("failed to upload part %d: %w", partNum, err)
		}

		state.Parts = append(state.Parts, types.CompletedPart{
			ETag:       uploadResp.ETag,
			PartNumber: aws.Int32(partNum),
		})
		state.Uploaded += size
		slog.Info("Uploaded part", "part", partNum, "uploaded", state.Uploaded, "total", fileSize)
	}

	// Complete the multipart upload
	_, err := s.client.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{
		Bucket:   aws.String(s.bucket),
		Key:      aws.String(key),
		UploadId: aws.String(state.UploadID),
		MultipartUpload: &types.CompletedMultipartUpload{
			Parts: state.Parts,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to complete multipart upload: %w", err)
	}

	// Clean up state
	delete(s.uploadStates, stateKey)
	slog.Info("Completed multipart upload", "bucket", s.bucket, "key", key)
	return nil
}
