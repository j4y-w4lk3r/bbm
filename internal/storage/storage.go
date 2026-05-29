// Package storage abstracts object-store operations behind a small
// interface so bbm can target B2 today and Wasabi/R2/S3 later by swapping
// only an endpoint URL.
//
// The interface intentionally exposes the union of "things B2's
// S3-compatible API supports natively" — list with prefix + cursor, get
// to a streaming reader, put from a streaming reader, delete by key.
// Multipart upload, copy, version listing, presigned URLs, and similar
// "later if useful" surface area is deliberately out of scope for v0.1.0.
package storage

import (
	"context"
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

// Backend is the abstraction over the object store. Every method takes
// a ctx — callers cancel uploads/downloads via context, not via custom
// stop channels.
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
}
