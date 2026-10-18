// Package miniostore implements usecase.ImageStore on MinIO (any S3-compatible
// object storage).
package miniostore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/comnarin/foodstore/internal/domain"
)

// requestTimeout bounds one image read so a stalled storage server cannot
// stall page loads; the caller then shows the default picture.
const requestTimeout = 3 * time.Second

// downCooldown is how long, after a connection failure, the store answers
// instantly with an error instead of making every request wait for the timeout
// again. Without it a stopped MinIO would delay every picture by the timeout.
const downCooldown = 10 * time.Second

var errTemporarilyDown = errors.New("object storage is temporarily unavailable")

type Store struct {
	client   *minio.Client
	bucket   string
	timeout  time.Duration
	cooldown time.Duration

	mu        sync.Mutex
	downUntil time.Time
}

// New builds a client. It does not contact the server, so the API starts and
// keeps working even when MinIO is down.
func New(endpoint, accessKey, secretKey, bucket string, useSSL bool) (*Store, error) {
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("create object storage client: %w", err)
	}
	return &Store{client: client, bucket: bucket, timeout: requestTimeout, cooldown: downCooldown}, nil
}

// Get reads and validates one picture. Anything that is not a small raster
// image is refused with domain.ErrInvalidImage.
func (s *Store) Get(ctx context.Context, key string) (domain.Image, error) {
	if s.isDown() {
		return domain.Image{}, errTemporarilyDown
	}
	callerCtx := ctx
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		s.noteFailure(callerCtx, err)
		return domain.Image{}, fmt.Errorf("get %s: %w", key, err)
	}
	defer obj.Close()

	info, err := obj.Stat()
	if err != nil {
		s.noteFailure(callerCtx, err)
		return domain.Image{}, fmt.Errorf("stat %s: %w", key, err)
	}
	if info.Size > domain.MaxImageBytes {
		return domain.Image{}, fmt.Errorf("%w: %s is %d bytes", domain.ErrInvalidImage, key, info.Size)
	}
	data, err := io.ReadAll(io.LimitReader(obj, domain.MaxImageBytes+1))
	if err != nil {
		s.noteFailure(callerCtx, err)
		return domain.Image{}, fmt.Errorf("read %s: %w", key, err)
	}

	contentType := info.ContentType
	if !strings.HasPrefix(contentType, "image/") {
		// Uploaded as application/octet-stream (common with scripts and some
		// tools): decide from the bytes themselves.
		contentType = http.DetectContentType(data)
	}
	return domain.NewImage(data, contentType)
}

// EnsureBucket creates the bucket if it does not exist. Call it at start-up as
// a courtesy; a failure is not fatal because the API serves defaults anyway.
func (s *Store) EnsureBucket(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	exists, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return fmt.Errorf("check bucket %s: %w", s.bucket, err)
	}
	if exists {
		return nil
	}
	if err := s.client.MakeBucket(ctx, s.bucket, minio.MakeBucketOptions{}); err != nil {
		return fmt.Errorf("create bucket %s: %w", s.bucket, err)
	}
	return nil
}

func (s *Store) isDown() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return time.Now().Before(s.downUntil)
}

// noteFailure starts the cooldown when the failure means "cannot reach the
// server" (timeout, refused, reset). An answer from the server, such as a
// missing object, is not an outage, and neither is the caller giving up.
func (s *Store) noteFailure(callerCtx context.Context, err error) {
	if callerCtx.Err() != nil {
		return
	}
	if minio.ToErrorResponse(err).Code != "" {
		return
	}
	s.mu.Lock()
	s.downUntil = time.Now().Add(s.cooldown)
	s.mu.Unlock()
}
