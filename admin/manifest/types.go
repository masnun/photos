package manifest

import "time"

type Manifest struct {
	Photos      []Photo      `json:"photos"`
	Genres      []Genre      `json:"genres"`
	Collections []Collection `json:"collections"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

type Photo struct {
	ID          string     `json:"id"`
	Filename    string     `json:"filename"`
	Caption     string     `json:"caption,omitempty"`
	Genres      []string   `json:"genres"`
	Collections []string   `json:"collections"`
	TakenAt     *time.Time `json:"taken_at,omitempty"`
	UploadedAt  time.Time  `json:"uploaded_at"`
	Width       int        `json:"width"`
	Height      int        `json:"height"`
	URLs        URLs       `json:"urls"`
	EXIF        *EXIF      `json:"exif,omitempty"`
	Lat         *float64   `json:"lat,omitempty"`
	Lon         *float64   `json:"lon,omitempty"`
	SourceHash  string     `json:"source_hash,omitempty"`
}

type URLs struct {
	Thumb string `json:"thumb"`
	Web   string `json:"web"`
	Full  string `json:"full"`
}

type EXIF struct {
	Camera      string `json:"camera,omitempty"`
	Lens        string `json:"lens,omitempty"`
	ISO         int    `json:"iso,omitempty"`
	Aperture    string `json:"aperture,omitempty"`
	Shutter     string `json:"shutter,omitempty"`
	FocalLength string `json:"focal_length,omitempty"`
}

type Genre struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Cover       string `json:"cover,omitempty"`
}

type Collection struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Cover       string `json:"cover,omitempty"`
}
