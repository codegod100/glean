package db

import (
	"context"
	"testing"

	"gotest.tools/v3/assert"
)

func TestCursorStore_LoadEmpty(t *testing.T) {
	ctx := context.Background()
	dbs := setupTestDB(t)
	store := dbs.CursorStore()

	cur, err := store.LoadCursor(ctx)
	assert.NilError(t, err)
	assert.Assert(t, cur == nil, "expected nil cursor for empty table")
}

func TestCursorStore_SaveAndLoad(t *testing.T) {
	ctx := context.Background()
	dbs := setupTestDB(t)
	store := dbs.CursorStore()

	err := store.SaveCursor(ctx, 1234567890)
	assert.NilError(t, err)

	cur, err := store.LoadCursor(ctx)
	assert.NilError(t, err)
	assert.Assert(t, cur != nil)
	assert.Equal(t, *cur, int64(1234567890))
}

func TestCursorStore_Update(t *testing.T) {
	ctx := context.Background()
	dbs := setupTestDB(t)
	store := dbs.CursorStore()

	assert.NilError(t, store.SaveCursor(ctx, 100))
	assert.NilError(t, store.SaveCursor(ctx, 200))

	cur, err := store.LoadCursor(ctx)
	assert.NilError(t, err)
	assert.Assert(t, cur != nil)
	assert.Equal(t, *cur, int64(200))
}
