package miniostore

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"net"
	"os"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/comnarin/foodstore/internal/domain"
	"github.com/comnarin/foodstore/internal/usecase"
)

var _ usecase.ImageStore = (*Store)(nil)

func TestGet_UnreachableServerFailsFastWithAnError(t *testing.T) {
	// Nothing listens on port 1, like MinIO being stopped.
	s, err := New("127.0.0.1:1", "key", "secret", "product-images", false)
	if err != nil {
		t.Fatal(err)
	}
	s.timeout = 2 * time.Second

	start := time.Now()
	_, err = s.Get(context.Background(), "red.png")

	if err == nil {
		t.Fatal("want an error when the server is unreachable")
	}
	if took := time.Since(start); took > 5*time.Second {
		t.Fatalf("took %v, an unreachable server must not stall requests", took)
	}
}

func TestGet_AfterATimeoutItDoesNotWaitAgainUntilTheCooldownEnds(t *testing.T) {
	// A server that accepts connections but never answers, like a hung MinIO.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			t.Cleanup(func() { _ = conn.Close() })
		}
	}()
	s, err := New(ln.Addr().String(), "key", "secret", "product-images", false)
	if err != nil {
		t.Fatal(err)
	}
	s.timeout = 300 * time.Millisecond
	s.cooldown = 600 * time.Millisecond

	took := func() time.Duration {
		start := time.Now()
		if _, err := s.Get(context.Background(), "red.png"); err == nil {
			t.Fatal("want an error from a server that never answers")
		}
		return time.Since(start)
	}

	if d := took(); d < 250*time.Millisecond {
		t.Fatalf("first call took %v, it should have waited for the timeout", d)
	}
	if d := took(); d > 100*time.Millisecond {
		t.Fatalf("second call took %v: while the storage is known to be down it must fail instantly", d)
	}
	time.Sleep(700 * time.Millisecond)
	if d := took(); d < 250*time.Millisecond {
		t.Fatalf("after the cooldown the storage must be tried again, call took %v", d)
	}
}

// --- integration tests: run only when a MinIO server is available ---------

type live struct {
	store  *Store
	client *minio.Client
	bucket string
}

func liveMinio(t *testing.T) live {
	t.Helper()
	endpoint := os.Getenv("MINIO_TEST_ENDPOINT")
	if endpoint == "" {
		t.Skip("MINIO_TEST_ENDPOINT not set; skipping MinIO integration test")
	}
	access, secret := os.Getenv("MINIO_TEST_ACCESS_KEY"), os.Getenv("MINIO_TEST_SECRET_KEY")
	bucket := "foodstore-test-images"

	client, err := minio.New(endpoint, &minio.Options{Creds: credentials.NewStaticV4(access, secret, "")})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if ok, _ := client.BucketExists(ctx, bucket); !ok {
		if err := client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	store, err := New(endpoint, access, secret, bucket, false)
	if err != nil {
		t.Fatal(err)
	}
	return live{store: store, client: client, bucket: bucket}
}

func (l live) put(t *testing.T, key string, data []byte, contentType string) {
	t.Helper()
	_, err := l.client.PutObject(context.Background(), l.bucket, key, bytes.NewReader(data), int64(len(data)),
		minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.client.RemoveObject(context.Background(), l.bucket, key, minio.RemoveObjectOptions{}) })
}

func tinyPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	img.Set(1, 1, color.RGBA{R: 200, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestGet_ReturnsAStoredImage(t *testing.T) {
	l := liveMinio(t)
	data := tinyPNG(t)
	l.put(t, "red.png", data, "image/png")

	got, err := l.store.Get(context.Background(), "red.png")
	if err != nil {
		t.Fatal(err)
	}
	if got.ContentType != "image/png" || !bytes.Equal(got.Data, data) {
		t.Fatalf("got %s, %d bytes", got.ContentType, len(got.Data))
	}
}

func TestGet_SniffsTheTypeWhenTheObjectIsStoredAsOctetStream(t *testing.T) {
	l := liveMinio(t)
	l.put(t, "blue.bin", tinyPNG(t), "application/octet-stream")

	got, err := l.store.Get(context.Background(), "blue.bin")
	if err != nil {
		t.Fatal(err)
	}
	if got.ContentType != "image/png" {
		t.Fatalf("ContentType = %q, want image/png (sniffed from the bytes)", got.ContentType)
	}
}

func TestGet_MissingObjectIsAnError(t *testing.T) {
	l := liveMinio(t)

	if _, err := l.store.Get(context.Background(), "no-such-file.png"); err == nil {
		t.Fatal("want an error for a missing object")
	}
}

func TestGet_RefusesFilesThatAreNotServableImages(t *testing.T) {
	l := liveMinio(t)
	l.put(t, "evil.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`), "image/svg+xml")
	l.put(t, "page.html", []byte("<script>alert(1)</script>"), "text/html")

	for _, key := range []string{"evil.svg", "page.html"} {
		if _, err := l.store.Get(context.Background(), key); !errors.Is(err, domain.ErrInvalidImage) {
			t.Fatalf("%s: err = %v, want ErrInvalidImage", key, err)
		}
	}
}

func TestEnsureBucket_CreatesItOnceAndIsRepeatable(t *testing.T) {
	l := liveMinio(t)
	s, err := New(os.Getenv("MINIO_TEST_ENDPOINT"), os.Getenv("MINIO_TEST_ACCESS_KEY"), os.Getenv("MINIO_TEST_SECRET_KEY"), "foodstore-test-ensure", false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.client.RemoveBucket(context.Background(), "foodstore-test-ensure") })

	for i := 0; i < 2; i++ {
		if err := s.EnsureBucket(context.Background()); err != nil {
			t.Fatalf("call %d: %v", i+1, err)
		}
	}
	if ok, _ := l.client.BucketExists(context.Background(), "foodstore-test-ensure"); !ok {
		t.Fatal("the bucket should exist")
	}
}

func TestGet_AMissingObjectIsNotAnOutage(t *testing.T) {
	l := liveMinio(t)
	l.put(t, "present.png", tinyPNG(t), "image/png")

	if _, err := l.store.Get(context.Background(), "absent.png"); err == nil {
		t.Fatal("want an error for a missing object")
	}

	// Had the miss started the cooldown, this next call would be refused.
	if _, err := l.store.Get(context.Background(), "present.png"); err != nil {
		t.Fatalf("a missing object must not make the whole storage look down: %v", err)
	}
}
