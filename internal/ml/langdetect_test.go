package ml

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

func TestIsKnownLanguage(t *testing.T) {
	assert.Assert(t, IsKnownLanguage("en"))
	assert.Assert(t, IsKnownLanguage("ja"))
	assert.Assert(t, IsKnownLanguage("zh"))
	assert.Assert(t, !IsKnownLanguage("xx"))
	assert.Assert(t, !IsKnownLanguage("bamboo-based plastic"))
	assert.Assert(t, !IsKnownLanguage(""))
}
