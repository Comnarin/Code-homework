package domain

import (
	"errors"
	"fmt"
	"mime"
	"strings"
)

// MaxImageBytes caps a product image so one oversized object cannot exhaust memory.
const MaxImageBytes = 5 << 20

var ErrInvalidImage = errors.New("invalid product image")

// servable lists the types that are safe to serve from our own origin.
// SVG and HTML are deliberately absent: they can carry scripts.
var servable = map[string]bool{
	"image/png":  true,
	"image/jpeg": true,
	"image/webp": true,
	"image/gif":  true,
}

// Image is the bytes of a product picture and how to serve them.
type Image struct {
	Data        []byte
	ContentType string
}

// NewImage validates untrusted bytes (for example from object storage) before
// they are served to a browser.
func NewImage(data []byte, contentType string) (Image, error) {
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(contentType))
	if err != nil {
		return Image{}, fmt.Errorf("%w: content type %q", ErrInvalidImage, contentType)
	}
	mediaType = strings.ToLower(mediaType)
	switch {
	case !servable[mediaType]:
		return Image{}, fmt.Errorf("%w: type %q is not allowed", ErrInvalidImage, mediaType)
	case len(data) == 0:
		return Image{}, fmt.Errorf("%w: empty", ErrInvalidImage)
	case len(data) > MaxImageBytes:
		return Image{}, fmt.Errorf("%w: larger than %d bytes", ErrInvalidImage, MaxImageBytes)
	}
	return Image{Data: data, ContentType: mediaType}, nil
}
