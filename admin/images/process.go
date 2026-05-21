package images

import (
	"bytes"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"

	"github.com/disintegration/imaging"
)

const (
	ThumbMax = 400
	WebMax   = 1600
)

type Variants struct {
	Thumb  []byte
	Web    []byte
	Full   []byte
	Width  int
	Height int
}

// ResizeForVision returns a JPEG bounded by maxDim on the long edge,
// suitable for sending to a vision API. Quality 85.
func ResizeForVision(data []byte, maxDim int) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	out := imaging.Fit(img, maxDim, maxDim, imaging.Lanczos)
	var buf bytes.Buffer
	if err := imaging.Encode(&buf, out, imaging.JPEG, imaging.JPEGQuality(85)); err != nil {
		return nil, fmt.Errorf("encode: %w", err)
	}
	return buf.Bytes(), nil
}

func Process(data []byte) (*Variants, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}

	b := img.Bounds()
	v := &Variants{
		Full:   data,
		Width:  b.Dx(),
		Height: b.Dy(),
	}

	thumb := imaging.Fit(img, ThumbMax, ThumbMax, imaging.Lanczos)
	var tbuf bytes.Buffer
	if err := imaging.Encode(&tbuf, thumb, imaging.JPEG, imaging.JPEGQuality(82)); err != nil {
		return nil, fmt.Errorf("encode thumb: %w", err)
	}
	v.Thumb = tbuf.Bytes()

	web := imaging.Fit(img, WebMax, WebMax, imaging.Lanczos)
	var wbuf bytes.Buffer
	if err := imaging.Encode(&wbuf, web, imaging.JPEG, imaging.JPEGQuality(88)); err != nil {
		return nil, fmt.Errorf("encode web: %w", err)
	}
	v.Web = wbuf.Bytes()

	return v, nil
}
