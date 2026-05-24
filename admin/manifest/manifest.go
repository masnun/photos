package manifest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Store struct {
	path string
	mu   sync.RWMutex
	data Manifest
}

func New(path string) (*Store, error) {
	s := &Store{path: path}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) load() error {
	f, err := os.Open(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
				return err
			}
			s.data = Manifest{
				Photos:      []Photo{},
				Genres:      []Genre{},
				Collections: []Collection{},
				UpdatedAt:   time.Now().UTC(),
			}
			return s.save()
		}
		return err
	}
	defer f.Close()
	if err := json.NewDecoder(f).Decode(&s.data); err != nil {
		return err
	}
	if s.data.Photos == nil {
		s.data.Photos = []Photo{}
	}
	if s.data.Genres == nil {
		s.data.Genres = []Genre{}
	}
	if s.data.Collections == nil {
		s.data.Collections = []Collection{}
	}
	return s.ensureFeatured()
}

// ensureFeatured guarantees the reserved "featured" collection exists.
// Photos tagged with this slug power the home-page slideshow.
func (s *Store) ensureFeatured() error {
	for _, c := range s.data.Collections {
		if c.Slug == FeaturedSlug {
			return nil
		}
	}
	s.data.Collections = append(s.data.Collections, Collection{
		Slug:        FeaturedSlug,
		Name:        "Featured",
		Description: "Highlights shown in the home-page slideshow.",
	})
	return s.save()
}

func (s *Store) save() error {
	tmp := s.path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	s.data.UpdatedAt = time.Now().UTC()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(&s.data); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *Store) Snapshot() Manifest {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data
}

func (s *Store) AddPhoto(p Photo) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Photos = append(s.data.Photos, p)
	return s.save()
}

func (s *Store) UpdatePhoto(id string, fn func(*Photo)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.data.Photos {
		if s.data.Photos[i].ID == id {
			fn(&s.data.Photos[i])
			return s.save()
		}
	}
	return fmt.Errorf("photo %s not found", id)
}

func (s *Store) DeletePhoto(id string) (Photo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, p := range s.data.Photos {
		if p.ID == id {
			s.data.Photos = append(s.data.Photos[:i], s.data.Photos[i+1:]...)
			return p, s.save()
		}
	}
	return Photo{}, fmt.Errorf("photo %s not found", id)
}

func (s *Store) UpsertGenre(g Genre) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.data.Genres {
		if s.data.Genres[i].Slug == g.Slug {
			if g.Order == nil {
				g.Order = s.data.Genres[i].Order
			}
			s.data.Genres[i] = g
			return s.save()
		}
	}
	s.data.Genres = append(s.data.Genres, g)
	return s.save()
}

func (s *Store) SetGenreOrder(slug string, order []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.data.Genres {
		if s.data.Genres[i].Slug == slug {
			s.data.Genres[i].Order = order
			return s.save()
		}
	}
	return fmt.Errorf("genre %s not found", slug)
}

// SetGenresOrder reorders the genre list to match the given slug order. Slugs
// not present in the manifest are ignored; any existing genres missing from the
// order list are appended in their current relative order so none are dropped.
func (s *Store) SetGenresOrder(order []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	bySlug := make(map[string]Genre, len(s.data.Genres))
	for _, g := range s.data.Genres {
		bySlug[g.Slug] = g
	}
	reordered := make([]Genre, 0, len(s.data.Genres))
	seen := make(map[string]bool, len(order))
	for _, slug := range order {
		if g, ok := bySlug[slug]; ok && !seen[slug] {
			reordered = append(reordered, g)
			seen[slug] = true
		}
	}
	for _, g := range s.data.Genres {
		if !seen[g.Slug] {
			reordered = append(reordered, g)
		}
	}
	s.data.Genres = reordered
	return s.save()
}

func (s *Store) SetGenreCover(slug, photoID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.data.Genres {
		if s.data.Genres[i].Slug == slug {
			s.data.Genres[i].Cover = photoID
			return s.save()
		}
	}
	return fmt.Errorf("genre %s not found", slug)
}

func (s *Store) SetCollectionCover(slug, photoID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.data.Collections {
		if s.data.Collections[i].Slug == slug {
			s.data.Collections[i].Cover = photoID
			return s.save()
		}
	}
	return fmt.Errorf("collection %s not found", slug)
}

func (s *Store) DeleteGenre(slug string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, g := range s.data.Genres {
		if g.Slug == slug {
			s.data.Genres = append(s.data.Genres[:i], s.data.Genres[i+1:]...)
			return s.save()
		}
	}
	return fmt.Errorf("genre %s not found", slug)
}

func (s *Store) UpsertCollection(c Collection) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.data.Collections {
		if s.data.Collections[i].Slug == c.Slug {
			if c.Order == nil {
				c.Order = s.data.Collections[i].Order
			}
			s.data.Collections[i] = c
			return s.save()
		}
	}
	s.data.Collections = append(s.data.Collections, c)
	return s.save()
}

func (s *Store) SetCollectionOrder(slug string, order []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.data.Collections {
		if s.data.Collections[i].Slug == slug {
			s.data.Collections[i].Order = order
			return s.save()
		}
	}
	return fmt.Errorf("collection %s not found", slug)
}

func (s *Store) DeleteCollection(slug string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, c := range s.data.Collections {
		if c.Slug == slug {
			s.data.Collections = append(s.data.Collections[:i], s.data.Collections[i+1:]...)
			return s.save()
		}
	}
	return fmt.Errorf("collection %s not found", slug)
}
