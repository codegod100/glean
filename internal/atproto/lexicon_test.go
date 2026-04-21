package atproto

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
)

func lexiconPath(filename string) string {
	return filepath.Join("..", "..", "lexicons", "at", "glean", filename)
}

func readLexiconProperties(t *testing.T, filename string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(lexiconPath(filename))
	assert.NilError(t, err)

	var schema struct {
		Defs struct {
			Main struct {
				Record struct {
					Properties map[string]any `json:"properties"`
				} `json:"record"`
			} `json:"main"`
		} `json:"defs"`
	}
	assert.NilError(t, json.Unmarshal(data, &schema))
	assert.Assert(t, len(schema.Defs.Main.Record.Properties) > 0, "no properties found in %s", filename)
	return schema.Defs.Main.Record.Properties
}

func assertStructMatchesLexicon[T any](t *testing.T, filename string) {
	t.Helper()
	properties := readLexiconProperties(t, filename)

	var zero T
	typ := reflect.TypeOf(zero)

	jsonTags := make(map[string]bool)
	for field := range typ.Fields() {
		tag := field.Tag.Get("json")
		name := strings.Split(tag, ",")[0]
		if name == "" || name == "-" {
			continue
		}
		jsonTags[name] = true
	}

	for prop := range properties {
		assert.Assert(t, jsonTags[prop], "lexicon property %q missing from %s (lexicon file: %s)", prop, typ.Name(), filename)
	}

	for name := range jsonTags {
		_, exists := properties[name]
		assert.Assert(t, exists, "Go field %q in %s missing from lexicon %s", name, typ.Name(), filename)
	}
}

func TestSubscriptionRecordMatchesLexicon(t *testing.T) {
	assertStructMatchesLexicon[SubscriptionRecord](t, "subscription.json")
}

func TestAnnotationRecordMatchesLexicon(t *testing.T) {
	assertStructMatchesLexicon[AnnotationRecord](t, "annotation.json")
}

func TestLikeRecordMatchesLexicon(t *testing.T) {
	assertStructMatchesLexicon[LikeRecord](t, "like.json")
}
