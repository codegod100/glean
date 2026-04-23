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
