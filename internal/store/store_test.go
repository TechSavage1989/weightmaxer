package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveLoadAndBackup(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sub", "data.json")
	s := New(p)
	if got, _ := s.Load(); got != "{}" {
		t.Fatalf("empty load = %q", got)
	}
	if err := s.Save(`{"a":1}`); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(`{"a":2}`); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Load(); got != `{"a":2}` {
		t.Fatalf("load = %q", got)
	}
	if b, _ := os.ReadFile(p + ".bak"); string(b) != `{"a":1}` {
		t.Fatalf("backup = %q", b)
	}
	// A damaged main file falls back to the previous copy.
	os.WriteFile(p, []byte("{broken"), 0o600)
	if got, _ := s.Load(); got != `{"a":1}` {
		t.Fatalf("fallback = %q", got)
	}
}

func TestRejectsBadData(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "d.json"))
	for _, bad := range []string{"[]", "not json", `"x"`, "{" + strings.Repeat(" ", MaxSize) + "}"} {
		if s.Save(bad) == nil {
			t.Fatalf("accepted %.20q", bad)
		}
	}
	if s.Busy() {
		t.Fatal("busy after failed saves")
	}
}
