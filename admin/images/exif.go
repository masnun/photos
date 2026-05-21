package images

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/rwcarlsen/goexif/exif"

	"github.com/masnun/photos/admin/manifest"
)

// ExtractEXIF parses EXIF metadata from image bytes. Returns (nil, nil, nil)
// when the image has no parsable EXIF — that is not an error condition for
// upload (PNGs, screenshots, exported web copies often lack it).
func ExtractEXIF(data []byte) (*manifest.EXIF, *time.Time, error) {
	x, err := exif.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, nil, nil
	}

	e := &manifest.EXIF{}

	make_ := tagString(x, exif.Make)
	model := tagString(x, exif.Model)
	switch {
	case make_ != "" && model != "" && !strings.HasPrefix(strings.ToLower(model), strings.ToLower(make_)):
		e.Camera = make_ + " " + model
	case model != "":
		e.Camera = model
	case make_ != "":
		e.Camera = make_
	}

	e.Lens = tagString(x, exif.LensModel)

	if t, err := x.Get(exif.ISOSpeedRatings); err == nil {
		if iso, err := t.Int(0); err == nil {
			e.ISO = iso
		}
	}

	if n, d, ok := tagRat(x, exif.FNumber); ok && d != 0 {
		e.Aperture = fmt.Sprintf("f/%.1f", float64(n)/float64(d))
	}

	if n, d, ok := tagRat(x, exif.ExposureTime); ok {
		e.Shutter = formatShutter(n, d)
	}

	if n, d, ok := tagRat(x, exif.FocalLength); ok && d != 0 {
		e.FocalLength = fmt.Sprintf("%.0fmm", float64(n)/float64(d))
	}

	var taken *time.Time
	if dt, err := x.DateTime(); err == nil {
		utc := dt.UTC()
		taken = &utc
	}

	if e.Camera == "" && e.Lens == "" && e.ISO == 0 && e.Aperture == "" && e.Shutter == "" && e.FocalLength == "" {
		e = nil
	}
	return e, taken, nil
}

func tagString(x *exif.Exif, name exif.FieldName) string {
	t, err := x.Get(name)
	if err != nil {
		return ""
	}
	s, err := t.StringVal()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(strings.Trim(s, "\x00"))
}

func tagRat(x *exif.Exif, name exif.FieldName) (int64, int64, bool) {
	t, err := x.Get(name)
	if err != nil {
		return 0, 0, false
	}
	n, d, err := t.Rat2(0)
	if err != nil {
		return 0, 0, false
	}
	return n, d, true
}

func formatShutter(n, d int64) string {
	if n == 0 || d == 0 {
		return ""
	}
	if n < d {
		return fmt.Sprintf("1/%d", d/n)
	}
	return fmt.Sprintf("%.1fs", float64(n)/float64(d))
}
