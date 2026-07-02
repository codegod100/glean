package server

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNonNil(t *testing.T) {
	var nilStrs []string
	if got := nonNil(nilStrs); got == nil || len(got) != 0 {
		t.Fatalf("nonNil(nil) = %v, want non-nil empty slice", got)
	}
	in := []string{"a"}
	if got := nonNil(in); &got[0] != &in[0] {
		t.Fatalf("nonNil returned a copy of a non-nil slice")
	}
}

// TestPtrTo confirms optional response fields serialize as null when unset
// and as the value when set via new().
func TestPtrTo(t *testing.T) {
	type holder struct {
		Feed *Feed `json:"feed,omitempty"`
	}
	rec := httptest.NewRecorder()
	writeJSON(rec, 200, holder{})
	if !strings.Contains(rec.Body.String(), `{}`) {
		t.Fatalf("empty optional should omit: %s", rec.Body.String())
	}

	rec2 := httptest.NewRecorder()
	writeJSON(rec2, 200, holder{Feed: new(Feed{FeedURL: "x"})})
	if !strings.Contains(rec2.Body.String(), `"feed_url":"x"`) {
		t.Fatalf("set optional should serialize: %s", rec2.Body.String())
	}
}

// TestAnnotation_NilReceiver_Tags ensures tags serialize as [] not null
// even for a nil-receiver conversion (the contract the frontend relies on).
func TestAnnotation_NilReceiver_Tags(t *testing.T) {
	b, err := json.Marshal(toAnnotation(nil))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(b), `"tags":[]`) {
		t.Fatalf("tags = %s, want []", b)
	}
}

func TestWriteAPIError(t *testing.T) {
	rec := httptest.NewRecorder()
	writeAPIError(rec, 400, "boom")
	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"error":"boom"`) {
		t.Fatalf("body = %s, want error:boom", rec.Body.String())
	}
}
