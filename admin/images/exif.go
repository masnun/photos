package images

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/rwcarlsen/goexif/exif"

	"github.com/masnun/photos/admin/manifest"
)

// ExtractEXIF parses EXIF metadata from image bytes. Returns zero values
// when the image has no parsable EXIF — that is not an error condition for
// upload (PNGs, screenshots, exported web copies often lack it).
func ExtractEXIF(data []byte) (*manifest.EXIF, *time.Time, *float64, *float64, error) {
	x, err := exif.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, nil, nil, nil, nil
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

	lat, lon := extractGPS(x)
	return e, taken, lat, lon, nil
}

func extractGPS(x *exif.Exif) (*float64, *float64) {
	lat, latOK := readGPSCoord(x, exif.GPSLatitude, exif.GPSLatitudeRef, "S")
	lon, lonOK := readGPSCoord(x, exif.GPSLongitude, exif.GPSLongitudeRef, "W")
	if !latOK || !lonOK {
		return nil, nil
	}
	if lat == 0 && lon == 0 {
		return nil, nil
	}
	return &lat, &lon
}

func readGPSCoord(x *exif.Exif, coord, ref exif.FieldName, negativeRef string) (float64, bool) {
	t, err := x.Get(coord)
	if err != nil {
		return 0, false
	}
	parts := make([]float64, 3)
	for i := 0; i < 3; i++ {
		n, d, err := t.Rat2(i)
		if err != nil || d == 0 {
			return 0, false
		}
		parts[i] = float64(n) / float64(d)
	}
	val := parts[0] + parts[1]/60 + parts[2]/3600
	if r, err := x.Get(ref); err == nil {
		if s, err := r.StringVal(); err == nil {
			s = strings.TrimSpace(strings.Trim(s, "\x00"))
			if strings.EqualFold(s, negativeRef) {
				val = -val
			}
		}
	}
	return val, true
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
