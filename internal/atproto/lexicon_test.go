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

func lexiconPathFromRoot(relPath string) string {
	return filepath.Join("..", "..", "lexicons", relPath)
}

func readLexiconPropertiesFromPath(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
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
	assert.Assert(t, len(schema.Defs.Main.Record.Properties) > 0, "no properties found in %s", path)
	return schema.Defs.Main.Record.Properties
}

func assertStructMatchesLexicon[T any](t *testing.T, filename string) {
	t.Helper()
	assertStructMatchesLexiconPath[T](t, lexiconPath(filename))
}

func assertStructMatchesLexiconPath[T any](t *testing.T, path string) {
	t.Helper()
	properties := readLexiconPropertiesFromPath(t, path)

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
		assert.Assert(t, jsonTags[prop], "lexicon property %q missing from %s (lexicon file: %s)", prop, typ.Name(), path)
	}

	for name := range jsonTags {
		_, exists := properties[name]
		assert.Assert(t, exists, "Go field %q in %s missing from lexicon %s", name, typ.Name(), path)
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

func TestFollowRecordMatchesLexicon(t *testing.T) {
	assertStructMatchesLexiconPath[FollowRecord](t, lexiconPathFromRoot("app/bsky/graph/follow.json"))
}

func TestMarginNoteRecordMatchesLexicon(t *testing.T) {
	assertStructMatchesLexiconPath[MarginNoteRecord](t, lexiconPathFromRoot("at/margin/note.json"))
}

func TestSkyreaderSubscriptionRecordMatchesLexicon(t *testing.T) {
	assertStructMatchesLexiconPath[SkyreaderSubscriptionRecord](t, lexiconPathFromRoot("app/skyreader/feed/subscription.json"))
}

func TestStandardPublicationRecordMatchesLexicon(t *testing.T) {
	assertStructMatchesLexiconPath[StandardPublicationRecord](t, lexiconPathFromRoot("site/standard/publication.json"))
}

func TestStandardDocumentRecordMatchesLexicon(t *testing.T) {
	assertStructMatchesLexiconPath[StandardDocumentRecord](t, lexiconPathFromRoot("site/standard/document.json"))
}
