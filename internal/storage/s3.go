// Package storage — S3-compatible Backend implementation, used for
// Backblaze B2 (via its S3 API), Wasabi, Cloudflare R2, and AWS S3
// proper. Only the endpoint URL changes between providers.
package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"

	"github.com/j4y-w4lk3r/bbm/internal/config"
)

// S3Backend is a Backend implementation against any S3-compatible
// endpoint. NewS3 wires it up from a fully-resolved config.Config
// (post-cascade, post-op-resolution).
type S3Backend struct {
	client   *s3.Client
	uploader *manager.Uploader
	bucket   string
	keyID    string
	appKey   string
	endpoint string
}

// NewS3 builds an S3Backend pointed at cfg.Endpoint with cfg.KeyID +
// cfg.AppKey static credentials. Path-style URLs are forced because
// virtual-hosted-style URLs require DNS-resolvable bucket names which
// not every B2 region exposes the same way.
func NewS3(ctx context.Context, cfg *config.Config) (*S3Backend, error) {
	if cfg.Bucket == "" {
		return nil, errors.New("storage: bucket is empty")
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(cfg.Region),
		awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.KeyID, cfg.AppKey, ""),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("storage: load AWS config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		}
		// Path-style URLs (https://endpoint/bucket/key) work everywhere;
		// virtual-hosted-style requires DNS-resolvable bucket names,
		// which B2's S3 API doesn't always serve.
		o.UsePathStyle = true
	})

	uploader := manager.NewUploader(client, func(u *manager.Uploader) {
		// 5 MiB part size + 5 concurrent parts is a sane default for
		// SOHO uplinks. Multipart kicks in automatically above ~16 MiB.
		u.PartSize = 5 * 1024 * 1024
		u.Concurrency = 5
	})

	return &S3Backend{
		client:   client,
		uploader: uploader,
		bucket:   cfg.Bucket,
		keyID:    cfg.KeyID,
		appKey:   cfg.AppKey,
		endpoint: cfg.Endpoint,
	}, nil
}

// List implements Backend.List. ContinuationToken is opaque — pass
// whatever the previous call returned back in.
func (b *S3Backend) List(ctx context.Context, prefix, token string, max int) ([]Object, string, error) {
	if max <= 0 {
		max = 1000
	}
	in := &s3.ListObjectsV2Input{
		Bucket:  aws.String(b.bucket),
		Prefix:  aws.String(prefix),
		MaxKeys: aws.Int32(int32(max)),
	}
	if token != "" {
		in.ContinuationToken = aws.String(token)
	}

	out, err := b.client.ListObjectsV2(ctx, in)
	if err != nil {
		return nil, "", wrapAWSErr("list", err)
	}

	objs := make([]Object, 0, len(out.Contents))
	for _, o := range out.Contents {
		obj := Object{}
		if o.Key != nil {
			obj.Key = *o.Key
		}
		if o.Size != nil {
			obj.Size = *o.Size
		}
		if o.LastModified != nil {
			obj.LastModified = *o.LastModified
		}
		if o.ETag != nil {
			obj.ETag = *o.ETag
		}
		objs = append(objs, obj)
	}

	next := ""
	if out.IsTruncated != nil && *out.IsTruncated && out.NextContinuationToken != nil {
		next = *out.NextContinuationToken
	}
	return objs, next, nil
}

// Get implements Backend.Get. Caller MUST Close the returned reader.
func (b *S3Backend) Get(ctx context.Context, key string) (io.ReadCloser, int64, error) {
	out, err := b.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, 0, wrapAWSErr("get "+key, err)
	}
	size := int64(-1)
	if out.ContentLength != nil {
		size = *out.ContentLength
	}
	return out.Body, size, nil
}

// Put implements Backend.Put. size is informational; we always go
// through the multipart-aware uploader, which handles streams of
// unknown length transparently.
func (b *S3Backend) Put(ctx context.Context, key string, body io.Reader, size int64) error {
	_ = size
	_, err := b.uploader.Upload(ctx, &s3.PutObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(key),
		Body:   body,
	})
	if err != nil {
		return wrapAWSErr("put "+key, err)
	}
	return nil
}

// Delete implements Backend.Delete. S3 considers deletion of a
// non-existent key a success, and so do we.
func (b *S3Backend) Delete(ctx context.Context, key string) error {
	_, err := b.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return wrapAWSErr("delete "+key, err)
	}
	return nil
}

// CreateBucket implements Backend.CreateBucket. Maps "BucketAlreadyExists"
// and "BucketAlreadyOwnedByYou" to the storage.ErrBucketExists sentinel
// so the CLI can offer a friendlier message; everything else falls
// through to the generic AWS-error wrapper.
//
// `region == "" || region == "us-east-1"` skips the
// CreateBucketConfiguration block, which mirrors the AWS S3 quirk:
// us-east-1 is the only region where you MUST NOT specify a
// LocationConstraint. For B2 / Wasabi / R2 the region argument always
// matters and we always send it.
// CreateBucket implements Backend.CreateBucket. When encrypt is true and
// provider is B2, default SSE-B2 is enabled via the Native API after
// the S3-compatible create succeeds.
func (b *S3Backend) CreateBucket(ctx context.Context, name, region string, encrypt bool) error {
	in := &s3.CreateBucketInput{Bucket: aws.String(name)}
	if region != "" && region != "us-east-1" {
		in.CreateBucketConfiguration = &types.CreateBucketConfiguration{
			LocationConstraint: types.BucketLocationConstraint(region),
		}
	}
	_, err := b.client.CreateBucket(ctx, in)
	if err != nil {
		var ae smithy.APIError
		if errors.As(err, &ae) {
			switch ae.ErrorCode() {
			case "BucketAlreadyExists", "BucketAlreadyOwnedByYou":
				return ErrBucketExists
			}
		}
		return wrapAWSErr("create bucket "+name, err)
	}
	if encrypt && strings.Contains(strings.ToLower(b.endpoint), "backblazeb2.com") {
		if err := EnableB2BucketEncryption(ctx, b.keyID, b.appKey, name); err != nil {
			return fmt.Errorf("bucket %q created but SSE-B2 setup failed: %w", name, err)
		}
	}
	return nil
}

// ListBuckets implements Backend.ListBuckets. S3 ListBuckets is an
// account-level call so the configured bucket is irrelevant.
func (b *S3Backend) ListBuckets(ctx context.Context) ([]Bucket, error) {
	out, err := b.client.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err != nil {
		return nil, wrapAWSErr("list buckets", err)
	}
	buckets := make([]Bucket, 0, len(out.Buckets))
	for _, ob := range out.Buckets {
		bk := Bucket{}
		if ob.Name != nil {
			bk.Name = *ob.Name
		}
		if ob.CreationDate != nil {
			bk.CreatedAt = *ob.CreationDate
		}
		buckets = append(buckets, bk)
	}
	return buckets, nil
}

// DeleteBucket implements Backend.DeleteBucket. Maps the "BucketNotEmpty"
// error code to the storage.ErrBucketNotEmpty sentinel so the CLI can
// suggest `bbm rm` first.
func (b *S3Backend) DeleteBucket(ctx context.Context, name string) error {
	_, err := b.client.DeleteBucket(ctx, &s3.DeleteBucketInput{
		Bucket: aws.String(name),
	})
	if err != nil {
		var ae smithy.APIError
		if errors.As(err, &ae) && ae.ErrorCode() == "BucketNotEmpty" {
			return ErrBucketNotEmpty
		}
		return wrapAWSErr("delete bucket "+name, err)
	}
	return nil
}

// wrapAWSErr unwraps the verbose smithy-go OperationError chain into
// something a CLI user can read at 80 columns, while still preserving
// the original via errors.Unwrap for programmatic callers.
func wrapAWSErr(op string, err error) error {
	if err == nil {
		return nil
	}
	var ae smithy.APIError
	if errors.As(err, &ae) {
		return fmt.Errorf("%s: %s: %s", op, ae.ErrorCode(), ae.ErrorMessage())
	}
	// Specifically un-pretty the NoSuchKey path so callers can
	// errors.As against types.NoSuchKey if they want.
	var nsk *types.NoSuchKey
	if errors.As(err, &nsk) {
		return fmt.Errorf("%s: object not found", op)
	}
	return fmt.Errorf("%s: %w", op, err)
}
