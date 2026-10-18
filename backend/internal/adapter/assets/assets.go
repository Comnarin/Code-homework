// Package assets holds files compiled into the binary, so they are always
// available even when every external service is down.
package assets

import (
	_ "embed"

	"github.com/comnarin/foodstore/internal/domain"
)

//go:embed no-image.svg
var noImageSVG []byte

// NoImage is the picture shown for a product that has no image, or whose image
// cannot be loaded. It is trusted content, so it skips domain.NewImage (which
// refuses SVG for untrusted uploads).
func NoImage() domain.Image {
	return domain.Image{Data: noImageSVG, ContentType: "image/svg+xml"}
}
