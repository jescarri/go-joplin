package s3

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// Client wraps the AWS S3 client for Joplin-compatible object storage.
// Supports official AWS S3 and S3-compatible storage (e.g. MinIO).
// Credentials MUST be provided via environment variables:
//   - AWS_ACCESS_KEY_ID or ACCESS_KEY_ID
//   - AWS_SECRET_ACCESS_KEY or SECRET_ACCESS_KEY
type Client struct {
	client *s3.Client
	bucket string
}

// Config holds S3 connection settings (from Joplin sync.8.* settings).
type Config struct {
	Bucket          string
	Region          string
	Endpoint        string // e.g. https://s3.amazonaws.com/ or https://minio:9000
	ForcePathStyle  bool
	AccessKeyID     string // overridden by env if set
	SecretAccessKey string // overridden by env if set
}

// NewClient creates an S3 client suitable for AWS S3 and S3-compatible storage (MinIO, etc.).
// Credentials follow Joplin config: env vars (AWS_ACCESS_KEY_ID / ACCESS_KEY_ID and secret) override
// config; otherwise sync.8.username and sync.8.password from the Joplin config are used.
func NewClient(ctx context.Context, cfg Config) (*Client, error) {
	accessKey := cfg.AccessKeyID
	if v := getEnvFirst("AWS_ACCESS_KEY_ID", "ACCESS_KEY_ID"); v != "" {
		accessKey = v
	}
	secretKey := cfg.SecretAccessKey
	if v := getEnvFirst("AWS_SECRET_ACCESS_KEY", "SECRET_ACCESS_KEY"); v != "" {
		secretKey = v
	}
	if accessKey == "" || secretKey == "" {
		return nil, fmt.Errorf("S3 credentials required: set env vars or sync.8.username and sync.8.password in config")
	}

	region := cfg.Region
	if region == "" {
		region = "us-east-1"
	}

	opts := []func(*config.LoadOptions) error{
		config.WithRegion(region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
	}

	if cfg.Endpoint != "" {
		// Normalize: strip trailing slash so path-style requests use /bucket, not //bucket
		endpoint := strings.TrimSuffix(cfg.Endpoint, "/")
		opts = append(opts, config.WithEndpointResolverWithOptions(aws.EndpointResolverWithOptionsFunc(
			func(service, region string, options ...interface{}) (aws.Endpoint, error) {
				return aws.Endpoint{
					URL:               endpoint,
					SigningRegion:     region,
					HostnameImmutable: true,
				}, nil
			},
		)))
	}

	awsCfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("load AWS config: %w", err)
	}

	s3Opts := []func(*s3.Options){}
	if cfg.ForcePathStyle {
		s3Opts = append(s3Opts, func(o *s3.Options) {
			o.UsePathStyle = true
		})
	}

	client := s3.NewFromConfig(awsCfg, s3Opts...)
	return &Client{client: client, bucket: cfg.Bucket}, nil
}

// ListObjects lists object keys in the bucket with optional prefix and continuation token.
// Returns keys, updated times (Unix ms), and next continuation token if truncated.
func (c *Client) ListObjects(ctx context.Context, prefix, continuationToken string, maxKeys int32) (keys []string, updatedTimes []int64, nextToken string, err error) {
	if maxKeys <= 0 {
		maxKeys = 1000
	}
	input := &s3.ListObjectsV2Input{
		Bucket:  aws.String(c.bucket),
		Prefix:  aws.String(prefix),
		MaxKeys: aws.Int32(maxKeys),
	}
	if continuationToken != "" {
		input.ContinuationToken = aws.String(continuationToken)
	}

	out, err := c.client.ListObjectsV2(ctx, input)
	if err != nil {
		return nil, nil, "", err
	}

	for _, obj := range out.Contents {
		if obj.Key == nil {
			continue
		}
		keys = append(keys, *obj.Key)
		var ts int64
		if obj.LastModified != nil {
			ts = obj.LastModified.UnixMilli()
		}
		updatedTimes = append(updatedTimes, ts)
	}
	nextToken = ""
	if out.NextContinuationToken != nil {
		nextToken = *out.NextContinuationToken
	}
	return keys, updatedTimes, nextToken, nil
}

// GetObject downloads an object by key. Returns nil, nil if the object does not exist.
func (c *Client) GetObject(ctx context.Context, key string) ([]byte, error) {
	out, err := c.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	defer out.Body.Close()
	return io.ReadAll(out.Body)
}

// PutObject uploads content as an object.
func (c *Client) PutObject(ctx context.Context, key string, body []byte) error {
	_, err := c.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
		Body:   bytes.NewReader(body),
	})
	return err
}

// DeleteObject deletes a single object.
func (c *Client) DeleteObject(ctx context.Context, key string) error {
	_, err := c.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(c.bucket),
		Key:   aws.String(key),
	})
	return err
}

// DeleteObjects deletes multiple objects.
func (c *Client) DeleteObjects(ctx context.Context, keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	objs := make([]types.ObjectIdentifier, len(keys))
	for i, k := range keys {
		objs[i] = types.ObjectIdentifier{Key: aws.String(k)}
	}
	_, err := c.client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
		Bucket: aws.String(c.bucket),
		Delete: &types.Delete{Objects: objs},
	})
	return err
}

// HeadObject returns whether the object exists.
func (c *Client) HeadObject(ctx context.Context, key string) (bool, error) {
	_, err := c.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if isNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func isNotFound(err error) bool {
	var nsk *types.NotFound
	return errors.As(err, &nsk)
}

func getEnvFirst(names ...string) string {
	for _, n := range names {
		if v := os.Getenv(n); v != "" {
			return v
		}
	}
	return ""
}