package db

import (
	"context"
	"testing"

	"gotest.tools/v3/assert"
)

func TestGetUser(t *testing.T) {
	ctx := context.Background()
	dbs := setupTestDB(t)

	u, err := dbs.Users.CreateUser(ctx, "did:test:profile")
	assert.NilError(t, err)
	assert.Equal(t, u.DID, "did:test:profile")

	got, err := dbs.Users.GetUser(ctx, "did:test:profile")
	assert.NilError(t, err)
	assert.Equal(t, got.DID, "did:test:profile")
}

func TestUpdateLanguages(t *testing.T) {
	ctx := context.Background()
	dbs := setupTestDB(t)

	_, err := dbs.Users.CreateUser(ctx, "did:test:langs")
	assert.NilError(t, err)

	err = dbs.Users.UpdateLanguages(ctx, "did:test:langs", []string{"en", "fr", "de"})
	assert.NilError(t, err)

	langs, err := dbs.Users.GetLanguages(ctx, "did:test:langs")
	assert.NilError(t, err)
	assert.DeepEqual(t, langs, []string{"en", "fr", "de"})
}

func TestGetLanguages_Empty(t *testing.T) {
	ctx := context.Background()
	dbs := setupTestDB(t)

	_, err := dbs.Users.CreateUser(ctx, "did:test:nolangs")
	assert.NilError(t, err)

	langs, err := dbs.Users.GetLanguages(ctx, "did:test:nolangs")
	assert.NilError(t, err)
	assert.Assert(t, langs == nil)
}

func TestGetLanguages_Set(t *testing.T) {
	ctx := context.Background()
	dbs := setupTestDB(t)

	_, err := dbs.Users.CreateUser(ctx, "did:test:langset")
	assert.NilError(t, err)

	err = dbs.Users.UpdateLanguages(ctx, "did:test:langset", []string{"en", "ja"})
	assert.NilError(t, err)

	langs, err := dbs.Users.GetLanguages(ctx, "did:test:langset")
	assert.NilError(t, err)
	assert.DeepEqual(t, langs, []string{"en", "ja"})
}

func TestGetLanguages_ClearToEmpty(t *testing.T) {
	ctx := context.Background()
	dbs := setupTestDB(t)

	_, err := dbs.Users.CreateUser(ctx, "did:test:clearlangs")
	assert.NilError(t, err)

	err = dbs.Users.UpdateLanguages(ctx, "did:test:clearlangs", []string{"en"})
	assert.NilError(t, err)

	err = dbs.Users.UpdateLanguages(ctx, "did:test:clearlangs", nil)
	assert.NilError(t, err)

	langs, err := dbs.Users.GetLanguages(ctx, "did:test:clearlangs")
	assert.NilError(t, err)
	assert.Assert(t, langs == nil)
}
