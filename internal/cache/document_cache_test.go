package cache

import (
	"testing"
	"time"

	"github.com/pillion/regula/internal/store"
)

func TestLatestDocumentCacheRoundTrip(t *testing.T) {
	c := NewLatestDocumentCache(200*time.Millisecond, 2)
	if c == nil {
		t.Fatal("expected cache to be created")
	}

	row := store.GetLatestPublishedDocumentVersionRow{DocumentKey: "privacy-policy", Version: "v1"}
	c.Set("privacy-policy|en|all", row)

	got, ok := c.Get("privacy-policy|en|all")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got.Version != "v1" {
		t.Fatalf("unexpected version: %s", got.Version)
	}
}

func TestLatestDocumentCacheExpires(t *testing.T) {
	c := NewLatestDocumentCache(25*time.Millisecond, 2)
	c.Set("privacy-policy|en|all", store.GetLatestPublishedDocumentVersionRow{Version: "v1"})

	time.Sleep(35 * time.Millisecond)

	if _, ok := c.Get("privacy-policy|en|all"); ok {
		t.Fatal("expected cache entry to expire")
	}
}

func TestLatestDocumentCacheInvalidatesPrefix(t *testing.T) {
	c := NewLatestDocumentCache(time.Minute, 4)
	c.Set("privacy-policy|en|all", store.GetLatestPublishedDocumentVersionRow{Version: "v1"})
	c.Set("privacy-policy|it|all", store.GetLatestPublishedDocumentVersionRow{Version: "v2"})
	c.Set("terms-of-service|en|all", store.GetLatestPublishedDocumentVersionRow{Version: "v1"})

	c.InvalidatePrefix("privacy-policy|")

	if _, ok := c.Get("privacy-policy|en|all"); ok {
		t.Fatal("expected privacy-policy entry to be invalidated")
	}
	if _, ok := c.Get("privacy-policy|it|all"); ok {
		t.Fatal("expected privacy-policy locale entry to be invalidated")
	}
	if _, ok := c.Get("terms-of-service|en|all"); !ok {
		t.Fatal("expected unrelated entry to remain")
	}
}
