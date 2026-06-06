package marketbot

import (
	"os"
	"path/filepath"
	"testing"
)

// loadCatalog must accept a path that points at the DIRECTORY containing
// item-data.json (e.g. the install/working dir), not only the file itself.
// Reading a directory as a file fails cryptically — "is a directory" on Linux,
// "Incorrect function" on Windows (issue #116) — so a directory path should
// resolve to item-data.json inside it.
func TestLoadCatalog_AcceptsDirectoryPath(t *testing.T) {
	dir := t.TempDir()
	const itemJSON = `{"items":{"Foo_Item":{"name":"Foo","category":"items/weapons","stack_max":10}}}`
	if err := os.WriteFile(filepath.Join(dir, "item-data.json"), []byte(itemJSON), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	t.Run("directory path resolves item-data.json inside it", func(t *testing.T) {
		cat, err := loadCatalog(dir)
		if err != nil {
			t.Fatalf("loadCatalog(dir): unexpected error: %v", err)
		}
		if len(cat) != 1 || cat[0].TemplateID != "Foo_Item" {
			t.Fatalf("got %+v, want exactly Foo_Item", cat)
		}
	})

	t.Run("explicit file path still works", func(t *testing.T) {
		cat, err := loadCatalog(filepath.Join(dir, "item-data.json"))
		if err != nil {
			t.Fatalf("loadCatalog(file): unexpected error: %v", err)
		}
		if len(cat) != 1 {
			t.Fatalf("got %d items, want 1", len(cat))
		}
	})

	t.Run("directory without item-data.json returns a normal not-found error", func(t *testing.T) {
		empty := t.TempDir()
		if _, err := loadCatalog(empty); err == nil {
			t.Fatal("expected an error for a directory lacking item-data.json, got nil")
		}
	})
}
