package db

import (
	"context"
	"testing"

	"gotest.tools/v3/assert"
)

func TestUpdateUserProfile_SetsFields(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)

	_, err := db.ExecContext(ctx, `INSERT INTO users (did, handle) VALUES (?, ?)`, "did:test:profile", "tester")
	assert.NilError(t, err)

	err = db.UpdateUserProfile(ctx, "did:test:profile", "Display Name", "https://cdn.bsky.app/img/avatar.png")
	assert.NilError(t, err)

	u, err := db.GetUser(ctx, "did:test:profile")
	assert.NilError(t, err)
	assert.Equal(t, u.DisplayName.String, "Display Name")
	assert.Equal(t, u.AvatarURL.String, "https://cdn.bsky.app/img/avatar.png")
}

func TestUpdateUserProfile_DoesNotOverwriteWithEmpty(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)

	_, err := db.ExecContext(ctx, `INSERT INTO users (did, handle, display_name, avatar_url) VALUES (?, ?, ?, ?)`,
		"did:test:profile2", "tester2", "Existing Name", "https://old.avatar/url")
	assert.NilError(t, err)

	err = db.UpdateUserProfile(ctx, "did:test:profile2", "", "")
	assert.NilError(t, err)

	u, err := db.GetUser(ctx, "did:test:profile2")
	assert.NilError(t, err)
	assert.Equal(t, u.DisplayName.String, "Existing Name")
	assert.Equal(t, u.AvatarURL.String, "https://old.avatar/url")
}
