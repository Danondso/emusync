package server

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dublin/emusync/internal/model"
)

func TestReadProfile_DefaultWhenMissing(t *testing.T) {
	st := NewStorage(t.TempDir(), 10)
	doc, err := st.ReadProfile()
	if err != nil {
		t.Fatal(err)
	}
	if doc.Version != 1 || len(doc.Emulators) == 0 {
		t.Fatalf("unexpected default profile: %+v", doc)
	}
}

func TestWriteProfile_roundTrip(t *testing.T) {
	st := NewStorage(t.TempDir(), 10)
	doc := &ProfileDocument{
		Version: 1,
		Emulators: []model.EmulatorConfig{
			{Name: "test-emu", ProcessNames: []string{"test"}, SavePaths: []string{"saves"}},
		},
	}
	if err := st.WriteProfile(doc); err != nil {
		t.Fatal(err)
	}
	got, err := st.ReadProfile()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Emulators) != 1 || got.Emulators[0].Name != "test-emu" {
		t.Fatalf("%+v", got)
	}
	if _, err := os.Stat(filepath.Join(st.DataDir(), "admin", "profile.json")); err != nil {
		t.Fatal(err)
	}
}
