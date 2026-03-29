package seed

import (
	"path/filepath"
	"testing"
)

func TestLoadManifest(t *testing.T) {
	manifest, baseDir, err := LoadManifest(filepath.Join("..", "..", "seed", "foundation.json"))
	if err != nil {
		t.Fatalf("expected manifest to load, got %v", err)
	}

	if len(manifest.Documents) < 4 {
		t.Fatalf("expected at least 4 documents, got %d", len(manifest.Documents))
	}
	if len(manifest.ConsentPurposes) < 3 {
		t.Fatalf("expected consent purposes to load")
	}
	if baseDir == "" {
		t.Fatal("expected non-empty base dir")
	}
}
