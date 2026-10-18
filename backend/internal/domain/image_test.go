package domain

import (
	"bytes"
	"errors"
	"testing"
)

func TestNewImage_AcceptsCommonRasterTypes(t *testing.T) {
	for _, ct := range []string{"image/png", "image/jpeg", "image/webp", "image/gif"} {
		t.Run(ct, func(t *testing.T) {
			img, err := NewImage([]byte("data"), ct)
			if err != nil {
				t.Fatal(err)
			}
			if img.ContentType != ct || string(img.Data) != "data" {
				t.Fatalf("got %+v", img)
			}
		})
	}
}

func TestNewImage_NormalizesTheContentType(t *testing.T) {
	img, err := NewImage([]byte("data"), " Image/PNG ; charset=binary")
	if err != nil {
		t.Fatal(err)
	}
	if img.ContentType != "image/png" {
		t.Fatalf("ContentType = %q, want image/png", img.ContentType)
	}
}

func TestNewImage_RejectsAnythingElse(t *testing.T) {
	tests := []struct {
		name        string
		data        []byte
		contentType string
	}{
		{"svg can carry scripts", []byte("<svg/>"), "image/svg+xml"},
		{"html", []byte("<script>alert(1)</script>"), "text/html"},
		{"missing type", []byte("data"), ""},
		{"empty data", nil, "image/png"},
		{"too large", bytes.Repeat([]byte("x"), MaxImageBytes+1), "image/png"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewImage(tt.data, tt.contentType); !errors.Is(err, ErrInvalidImage) {
				t.Fatalf("err = %v, want ErrInvalidImage", err)
			}
		})
	}
}

func TestNewImage_AcceptsExactlyTheSizeLimit(t *testing.T) {
	if _, err := NewImage(bytes.Repeat([]byte("x"), MaxImageBytes), "image/png"); err != nil {
		t.Fatalf("an image of exactly MaxImageBytes should be accepted: %v", err)
	}
}
