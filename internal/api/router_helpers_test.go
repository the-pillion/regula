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
