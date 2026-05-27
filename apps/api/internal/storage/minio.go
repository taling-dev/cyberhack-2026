package storage

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const QCImagesBucket = "simaops-qc-images"

type MinIOClient struct {
	client *minio.Client
}

func NewMinIOClient() (*MinIOClient, error) {
	endpoint := os.Getenv("MINIO_ENDPOINT")
	if endpoint == "" {
		endpoint = "localhost:9000"
	}
	accessKey := os.Getenv("MINIO_ACCESS_KEY")
	if accessKey == "" {
		accessKey = "simaops"
	}
	secretKey := os.Getenv("MINIO_SECRET_KEY")
	if secretKey == "" {
		secretKey = "simaops-dev-secret"
	}
	useSSL := os.Getenv("MINIO_USE_SSL") == "true"

	client, err := minio.New(endpoint, &minio.Options{
		Creds:        credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure:       useSSL,
		BucketLookup: minio.BucketLookupPath, // ingress-friendly: /bucket/key not bucket.host/key
	})
	if err != nil {
		return nil, fmt.Errorf("minio client: %w", err)
	}
	return &MinIOClient{client: client}, nil
}

// PresignedPutURL generates a presigned PUT URL for uploading an object.
func (m *MinIOClient) PresignedPutURL(ctx context.Context, bucket, objectKey, contentType string, expiry time.Duration) (string, error) {
	reqParams := make(url.Values)
	reqParams.Set("Content-Type", contentType)

	u, err := m.client.PresignedPutObject(ctx, bucket, objectKey, expiry)
	if err != nil {
		return "", fmt.Errorf("presigned put: %w", err)
	}
	return u.String(), nil
}

// PresignedGetURL generates a presigned GET URL for viewing an object.
func (m *MinIOClient) PresignedGetURL(ctx context.Context, bucket, objectKey string, expiry time.Duration) (string, error) {
	u, err := m.client.PresignedGetObject(ctx, bucket, objectKey, expiry, nil)
	if err != nil {
		return "", fmt.Errorf("presigned get: %w", err)
	}
	return u.String(), nil
}

// Ping checks MinIO connectivity.
func (m *MinIOClient) Ping(ctx context.Context) error {
	_, err := m.client.ListBuckets(ctx)
	return err
}
