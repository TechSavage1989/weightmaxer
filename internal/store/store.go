// Package store saves the user's plan, weigh-in log and settings as one JSON
// document with atomic replacement and one previous copy.
package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// MaxSize bounds the saved document so a runaway page cannot fill the disk.
const MaxSize = 4 << 20

// Store reads and writes one JSON file.
type Store struct {
	path string
	mu   sync.Mutex
	busy bool
}

func New(path string) *Store { return &Store{path: path} }

// Load returns the saved document, falling back to the previous copy if the
// main file is missing or damaged. It returns "{}" when nothing was saved yet.
func (s *Store) Load() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range []string{s.path, s.path + ".bak"} {
		b, err := os.ReadFile(p)
		if err == nil && len(b) <= MaxSize && isObject(b) {
			return string(b), nil
		}
	}
	return "{}", nil
}

// Save replaces the document atomically and keeps the previous version as .bak.
func (s *Store) Save(doc string) error {
	b := []byte(doc)
	if len(b) > MaxSize {
		return fmt.Errorf("data is larger than %d MB", MaxSize>>20)
	}
	if !isObject(b) {
		return errors.New("data must be a JSON object")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.busy = true
	defer func() { s.busy = false }()
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err = f.Write(b); err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp)
		return err
	}
	if old, err := os.ReadFile(s.path); err == nil && isObject(old) && !bytes.Equal(old, b) {
		_ = os.WriteFile(s.path+".bak", old, 0o600)
	}
	return os.Rename(tmp, s.path)
}

// Busy reports whether a save is in progress.
func (s *Store) Busy() bool {
	if !s.mu.TryLock() {
		return true
	}
	defer s.mu.Unlock()
	return s.busy
}

func isObject(b []byte) bool {
	var v map[string]json.RawMessage
	return json.Unmarshal(b, &v) == nil
}
