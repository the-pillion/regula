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
	if len(manifest.Processors) < 3 {
		t.Fatalf("expected processor registry seeds to load")
	}
	if len(manifest.RetentionPolicies) < 3 {
		t.Fatalf("expected retention policy seeds to load")
	}
	if len(manifest.ProcessingActivities) < 3 {
		t.Fatalf("expected processing activity seeds to load")
	}
	if len(manifest.DPIARecords) < 1 {
		t.Fatalf("expected dpia record seeds to load")
	}
	if baseDir == "" {
		t.Fatal("expected non-empty base dir")
	}
}
