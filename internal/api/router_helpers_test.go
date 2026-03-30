package api

import "testing"

func TestNormalizeContentType(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{input: "", want: "html"},
		{input: "html", want: "html"},
		{input: "text/html", want: "html"},
		{input: "markdown", want: "markdown"},
		{input: "md", want: "markdown"},
		{input: "text/markdown", want: "markdown"},
	}

	for _, tc := range cases {
		got, err := normalizeContentType(tc.input)
		if err != nil {
			t.Fatalf("expected no error for %q, got %v", tc.input, err)
		}
		if got != tc.want {
			t.Fatalf("expected %q for %q, got %q", tc.want, tc.input, got)
		}
	}
}

func TestNormalizeContentTypeRejectsUnsupportedValues(t *testing.T) {
	if _, err := normalizeContentType("pdf"); err == nil {
		t.Fatal("expected unsupported content type to fail")
	}
}

func TestBuildLatestVersionCacheKey(t *testing.T) {
	got := buildLatestVersionCacheKey(" Privacy-Policy ", "EN", "All")
	if got != "privacy-policy|en|all" {
		t.Fatalf("unexpected cache key: %s", got)
	}
}

func TestNormalizeRelationshipType(t *testing.T) {
	if got := normalizeRelationshipType("subprocessor"); got != "subprocessor" {
		t.Fatalf("expected subprocessor, got %s", got)
	}
	if got := normalizeRelationshipType("anything-else"); got != "processor" {
		t.Fatalf("expected processor fallback, got %s", got)
	}
}

func TestNormalizeDPAStatus(t *testing.T) {
	if got := normalizeDPAStatus("signed"); got != "signed" {
		t.Fatalf("expected signed, got %s", got)
	}
	if got := normalizeDPAStatus("weird"); got != "unknown" {
		t.Fatalf("expected unknown fallback, got %s", got)
	}
}

func TestNormalizeDPIAStatus(t *testing.T) {
	if got := normalizeDPIAStatus("approved"); got != "approved" {
		t.Fatalf("expected approved, got %s", got)
	}
	if got := normalizeDPIAStatus("odd"); got != "draft" {
		t.Fatalf("expected draft fallback, got %s", got)
	}
}

func TestNormalizeRiskLevel(t *testing.T) {
	if got := normalizeRiskLevel("very_high"); got != "very_high" {
		t.Fatalf("expected very_high, got %s", got)
	}
	if got := normalizeRiskLevel("odd"); got != "medium" {
		t.Fatalf("expected medium fallback, got %s", got)
	}
}
