package sys1

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

func TestModelsDecodesReleaseDate(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %q, want GET", r.Method)
		}
		if r.URL.Path != "/v1/models" {
			t.Errorf("path = %q, want /v1/models", r.URL.Path)
		}
		writeJSON(w, http.StatusOK, `{
			"models": [
				{"name": "jev-latest", "description": "General-purpose model.", "release_date": "2026-09-15"}
			]
		}`, nil)
	})
	models, err := c.Models(context.Background())
	if err != nil {
		t.Fatalf("Models: %v", err)
	}
	if len(models) != 1 {
		t.Fatalf("len(models) = %d, want 1", len(models))
	}
	m := models[0]
	if m.Name != "jev-latest" {
		t.Errorf("Name = %q, want jev-latest", m.Name)
	}
	want := time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC)
	if !m.ReleaseDate.Equal(want) {
		t.Errorf("ReleaseDate = %v, want %v", m.ReleaseDate, want)
	}
}

func TestModelReleaseDateMissingOrEmpty(t *testing.T) {
	for _, body := range []string{
		`{"name": "jev-latest", "description": "General-purpose model."}`,
		`{"name": "jev-latest", "description": "General-purpose model.", "release_date": ""}`,
	} {
		var m Model
		if err := json.Unmarshal([]byte(body), &m); err != nil {
			t.Fatalf("Unmarshal(%s): %v", body, err)
		}
		if !m.ReleaseDate.IsZero() {
			t.Errorf("ReleaseDate = %v, want zero", m.ReleaseDate)
		}
	}
}

func TestModelReleaseDateMalformedErrors(t *testing.T) {
	var m Model
	body := `{"name": "jev-latest", "description": "General-purpose model.", "release_date": "not-a-date"}`
	if err := json.Unmarshal([]byte(body), &m); err == nil {
		t.Fatal("Unmarshal() = nil, want error for a malformed release_date")
	}
}

func TestModelMarshalJSONOmitsZeroReleaseDate(t *testing.T) {
	m := Model{Name: "jev-latest", Description: "General-purpose model."}
	got, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	assertJSONEqual(t, got, `{"name": "jev-latest", "description": "General-purpose model."}`)

	m.ReleaseDate = time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC)
	got, err = json.Marshal(m)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	assertJSONEqual(t, got, `{"name": "jev-latest", "description": "General-purpose model.", "release_date": "2026-09-15"}`)
}
