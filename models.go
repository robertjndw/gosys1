package sys1

import (
	"encoding/json"
	"fmt"
	"time"
)

// modelReleaseDateLayout is the date-only format the API uses for a
// model's release_date field.
const modelReleaseDateLayout = "2006-01-02"

// Model describes a model or alias available to the authenticated
// account, as returned by Client.Models.
type Model struct {
	// Name is the model name or alias accepted by a request's model
	// field.
	Name string `json:"name"`
	// Description is a human-readable description of the model.
	Description string `json:"description"`
	// ReleaseDate is the model's release date, parsed from the API's
	// "release_date" (YYYY-MM-DD) field.
	ReleaseDate time.Time `json:"-"`
}

// modelWire is the JSON shape of a Model on the wire, where
// release_date is a plain string.
type modelWire struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	ReleaseDate string `json:"release_date"`
}

// UnmarshalJSON implements json.Unmarshaler, parsing the wire
// "release_date" string into ReleaseDate. The API guarantees the
// YYYY-MM-DD format, so a malformed date is treated as an error rather
// than silently zeroed.
func (m *Model) UnmarshalJSON(b []byte) error {
	var wire modelWire
	if err := json.Unmarshal(b, &wire); err != nil {
		return err
	}
	t, err := time.Parse(modelReleaseDateLayout, wire.ReleaseDate)
	if err != nil {
		return fmt.Errorf("sys1: parsing model release_date %q: %w", wire.ReleaseDate, err)
	}
	m.Name = wire.Name
	m.Description = wire.Description
	m.ReleaseDate = t
	return nil
}

// MarshalJSON implements json.Marshaler, the inverse of UnmarshalJSON.
func (m Model) MarshalJSON() ([]byte, error) {
	return json.Marshal(modelWire{
		Name:        m.Name,
		Description: m.Description,
		ReleaseDate: m.ReleaseDate.Format(modelReleaseDateLayout),
	})
}

// modelsResponse is the wire shape of a GET /v1/models response body.
// Client.Models decodes it internally and returns the []Model slice
// directly.
type modelsResponse struct {
	Models []Model `json:"models"`
}
