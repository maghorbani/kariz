package artifact

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/kariz/kariz/internal/models"
)

// defaultPresignExpiry is the default expiry duration for presigned download URLs.
const defaultPresignExpiry = 1 * time.Hour

// S3ArtifactStore stores artifacts in an S3-compatible bucket (MinIO or AWS S3)
// using the minio-go SDK.
type S3ArtifactStore struct {
	client     *minio.Client
	bucket     string
	pathPrefix string
}

// NewS3ArtifactStore creates a new S3ArtifactStore connected to the given endpoint.
// It initialises the minio-go client and ensures the target bucket exists.
func NewS3ArtifactStore(endpoint, bucket, pathPrefix, region, accessKey, secretKey string, useSSL bool) (*S3ArtifactStore, error) {
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
		Region: region,
	})
	if err != nil {
		return nil, fmt.Errorf("create minio client: %w", err)
	}

	// Ensure the bucket exists; create it if not.
	ctx := context.Background()
	exists, err := client.BucketExists(ctx, bucket)
	if err != nil {
		return nil, fmt.Errorf("check bucket existence: %w", err)
	}
	if !exists {
		if err := client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{Region: region}); err != nil {
			return nil, fmt.Errorf("create bucket %s: %w", bucket, err)
		}
	}

	return &S3ArtifactStore{
		client:     client,
		bucket:     bucket,
		pathPrefix: pathPrefix,
	}, nil
}

// Store uploads an artifact to the S3-compatible bucket at
// {pathPrefix}/{executionID}/{fileName}.
func (s *S3ArtifactStore) Store(ctx context.Context, executionID string, label string, fileName string, content io.Reader) (*StoredArtifact, error) {
	objectKey := s.objectKey(executionID, fileName)
	contentType := detectContentType(fileName)

	info, err := s.client.PutObject(ctx, s.bucket, objectKey, content, -1, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return nil, fmt.Errorf("upload artifact to S3: %w", err)
	}

	return &StoredArtifact{
		StoragePath:   objectKey,
		StorageType:   string(models.DestS3),
		FileSizeBytes: info.Size,
		ContentType:   contentType,
	}, nil
}

// GetDownloadURL generates a presigned URL for downloading the artifact.
// The URL expires after defaultPresignExpiry (1 hour).
func (s *S3ArtifactStore) GetDownloadURL(ctx context.Context, storagePath string, storageType models.ArtifactDestType) (string, error) {
	reqParams := make(url.Values)
	presignedURL, err := s.client.PresignedGetObject(ctx, s.bucket, storagePath, defaultPresignExpiry, reqParams)
	if err != nil {
		return "", fmt.Errorf("generate presigned URL: %w", err)
	}
	return presignedURL.String(), nil
}

// Delete removes an artifact object from the S3-compatible bucket.
func (s *S3ArtifactStore) Delete(ctx context.Context, storagePath string, storageType models.ArtifactDestType) error {
	if err := s.client.RemoveObject(ctx, s.bucket, storagePath, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("delete artifact from S3: %w", err)
	}
	return nil
}

// objectKey builds the full object key for an artifact in the bucket.
func (s *S3ArtifactStore) objectKey(executionID, fileName string) string {
	if s.pathPrefix != "" {
		return s.pathPrefix + "/" + executionID + "/" + fileName
	}
	return executionID + "/" + fileName
}
