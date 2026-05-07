package langdetect

import (
	"testing"

	"gotest.tools/v3/assert"
)

func TestKnownLanguages(t *testing.T) {
	langs := KnownLanguages()
	assert.Assert(t, len(langs) > 0)
	found := false
	for _, l := range langs {
		if l.Code == "en" {
			found = true
			assert.Equal(t, l.Name, "English")
		}
	}
	assert.Assert(t, found)
}
