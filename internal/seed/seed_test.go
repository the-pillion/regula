package seed

import (
	"os"
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

func TestApplyPlaceholders(t *testing.T) {
	vars := map[string]string{"legal_name": "Acme S.r.l.", "vat_number": ""}

	rendered, err := applyPlaceholders("{{ legal_name }} — P.IVA {{vat_number}}", vars)
	if err != nil {
		t.Fatalf("expected substitution to succeed, got %v", err)
	}
	if rendered != "Acme S.r.l. — P.IVA " {
		t.Fatalf("unexpected render: %q", rendered)
	}

	if _, err := applyPlaceholders("{{ legal_nme }}", vars); err == nil {
		t.Fatal("expected unknown placeholder to fail loud")
	}
}

func TestLoadCompanyVars(t *testing.T) {
	baseDir := filepath.Join("..", "..", "seed")

	en, err := loadCompanyVars(baseDir, "en")
	if err != nil {
		t.Fatalf("load en vars: %v", err)
	}
	if _, ok := en["legal_name"]; !ok {
		t.Fatal("expected legal_name from company.base.json")
	}

	it, err := loadCompanyVars(baseDir, "it-IT")
	if err != nil {
		t.Fatalf("load it vars from BCP-47 locale: %v", err)
	}
	if en["legal_form"] == it["legal_form"] {
		t.Fatal("expected language overlay to differentiate legal_form")
	}
}

func TestSeedDocumentsRenderWithoutUnknownPlaceholders(t *testing.T) {
	baseDir := filepath.Join("..", "..", "seed")
	manifest, _, err := LoadManifest(filepath.Join(baseDir, "foundation.json"))
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}

	for _, doc := range manifest.Documents {
		content, err := os.ReadFile(filepath.Join(baseDir, doc.ContentPath))
		if err != nil {
			t.Fatalf("read %s: %v", doc.ContentPath, err)
		}
		vars, err := loadCompanyVars(baseDir, doc.Locale)
		if err != nil {
			t.Fatalf("vars for %s: %v", doc.ContentPath, err)
		}
		if _, err := applyPlaceholders(string(content), vars); err != nil {
			t.Fatalf("document %s has unresolved placeholders: %v", doc.ContentPath, err)
		}
	}
}
