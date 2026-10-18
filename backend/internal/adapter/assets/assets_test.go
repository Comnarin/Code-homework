package assets

import (
	"bytes"
	"strings"
	"testing"
)

func TestNoImage_IsASmallSafeSVG(t *testing.T) {
	img := NoImage()

	if img.ContentType != "image/svg+xml" {
		t.Fatalf("ContentType = %q", img.ContentType)
	}
	if !bytes.HasPrefix(img.Data, []byte("<svg")) {
		t.Fatalf("not an svg: %.40s", img.Data)
	}
	if len(img.Data) == 0 || len(img.Data) > 4096 {
		t.Fatalf("size = %d bytes, want a small non-empty file", len(img.Data))
	}
	lower := strings.ToLower(string(img.Data))
	for _, bad := range []string{"<script", "onload", "onerror", "javascript:", "<foreignobject", "<image", "href="} {
		if strings.Contains(lower, bad) {
			t.Fatalf("the default picture must not contain %q", bad)
		}
	}
}
