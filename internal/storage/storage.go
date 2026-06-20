// Package storage abstracts object-store operations behind a small
// interface so bbm can target B2 today and Wasabi/R2/S3 later by swapping
// only an endpoint URL.
//
// The interface intentionally exposes the union of "things B2's
// S3-compatible API supports natively" — list with prefix + cursor, get
// to a streaming reader, put from a streaming reader, delete by key,
// plus account-level bucket admin (create/list/delete). Multipart
// upload, copy, version listing, presigned URLs, and similar "later if
// useful" surface area is deliberately out of scope.
package storage

import (
	"context"
	"errors"
	"io"
	"time"
)

// Object describes a single key in the bucket — the metadata `bbm ls`
// renders. Sizes/timestamps are the values returned by the backend; we
// don't normalize past trimming `obj.Key` of any leading slash.
type Object struct {
	Key          string
	Size         int64
	LastModified time.Time
	ETag         string
}

// Bucket describes a single bucket the credentials can see. Returned
// by `bbm bucket list`.
type Bucket struct {
	Name      string
	CreatedAt time.Time
}

// ErrBucketExists is returned by CreateBucket when the requested name
// is already taken — either by your account or, for AWS S3 proper,
// globally. CLI callers translate it into a friendlier message.
var ErrBucketExists = errors.New("bucket already exists")

// ErrBucketNotEmpty is returned by DeleteBucket when the target bucket
// still has objects. S3 itself returns this; we surface a stable
// sentinel so the CLI can suggest `bbm rm` first.
var ErrBucketNotEmpty = errors.New("bucket is not empty")

// Backend is the abstraction over the object store. Every method takes
// a ctx — callers cancel uploads/downloads via context, not via custom
// stop channels.
//
// Object methods (List/Get/Put/Delete) all operate on the bucket the
// backend was constructed with (typically `cfg.Bucket`). Bucket admin
// methods (CreateBucket/ListBuckets/DeleteBucket) take the bucket name
// as an argument and are independent of the backend's home bucket —
// `bbm bucket create my-new-bucket` is meant to work even if the
// configured bucket doesn't exist yet.
type Backend interface {
	// List returns one page of objects whose key begins with prefix.
	// continuationToken: empty string for the first page; pass back the
	// returned nextToken to fetch the next page. nextToken is "" when
	// the listing is exhausted.
	List(ctx context.Context, prefix string, continuationToken string, max int) (objects []Object, nextToken string, err error)

	// Get streams the object body. The returned ReadCloser MUST be
	// closed by the caller. size is the Content-Length the server
	// advertised (-1 if unknown).
	Get(ctx context.Context, key string) (body io.ReadCloser, size int64, err error)

	// Put streams body into the bucket as `key`. `size` is the known
	// content length; pass -1 for streams of unknown length, in which
	// case the implementation may buffer or use multipart.
	Put(ctx context.Context, key string, body io.Reader, size int64) error

	// Delete removes a single key (the latest version, if versioning is
	// on). Returns nil if the key didn't exist (S3 semantics).
	Delete(ctx context.Context, key string) error

	// CreateBucket creates a new bucket. If region is empty, the
	// backend's default region is used. encrypt enables default SSE-B2
	// on B2 (no-op on other providers). Returns ErrBucketExists when
	// the name is already taken.
	CreateBucket(ctx context.Context, name, region string, encrypt bool) error

	// ListBuckets returns every bucket the credentials can see. The
	// list is global to the account, not scoped to the configured
	// bucket — i.e. you can run `bbm bucket list` on any bbm config to
	// see what other buckets exist.
	ListBuckets(ctx context.Context) ([]Bucket, error)

	// DeleteBucket removes an empty bucket. Returns ErrBucketNotEmpty
	// if the bucket still has objects (S3 refuses to remove a
	// non-empty bucket; clear it via `bbm rm` first).
	DeleteBucket(ctx context.Context, name string) error
}
