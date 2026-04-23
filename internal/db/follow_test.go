package db

import (
	"context"
	"testing"

	"gotest.tools/v3/assert"
)

func seedFollowData(t *testing.T, ctx context.Context, dbs *Databases) (userDID, targetDID string) {
	t.Helper()

	userDID = "did:test:follower"
	targetDID = "did:test:followed"

	_, err := dbs.DB().ExecContext(ctx, `INSERT INTO users (did) VALUES (?)`, userDID)
	assert.NilError(t, err)
	_, err = dbs.DB().ExecContext(ctx, `INSERT INTO users (did) VALUES (?)`, targetDID)
	assert.NilError(t, err)

	return userDID, targetDID
}

func TestUpsertFollow(t *testing.T) {
	ctx := context.Background()
	dbs := setupTestDB(t)
	userDID, targetDID := seedFollowData(t, ctx, dbs)

	err := dbs.Users.UpsertFollow(ctx, userDID, targetDID, "at://did:test:follower/app.bsky.graph.follow/123", "cid123")
	assert.NilError(t, err)

	following, err := dbs.Users.IsFollowing(ctx, userDID, targetDID)
	assert.NilError(t, err)
	assert.Equal(t, following, true)
}

func TestIsFollowing_NotFollowing(t *testing.T) {
	ctx := context.Background()
	dbs := setupTestDB(t)
	userDID, targetDID := seedFollowData(t, ctx, dbs)

	following, err := dbs.Users.IsFollowing(ctx, userDID, targetDID)
	assert.NilError(t, err)
	assert.Equal(t, following, false)
}

func TestDeleteFollow(t *testing.T) {
	ctx := context.Background()
	dbs := setupTestDB(t)
	userDID, targetDID := seedFollowData(t, ctx, dbs)

	err := dbs.Users.UpsertFollow(ctx, userDID, targetDID, "at://uri", "cid")
	assert.NilError(t, err)

	err = dbs.Users.DeleteFollow(ctx, userDID, targetDID)
	assert.NilError(t, err)

	following, err := dbs.Users.IsFollowing(ctx, userDID, targetDID)
	assert.NilError(t, err)
	assert.Equal(t, following, false)
}

func TestDeleteFollowByURI(t *testing.T) {
	ctx := context.Background()
	dbs := setupTestDB(t)
	userDID, targetDID := seedFollowData(t, ctx, dbs)

	uri := "at://did:test:follower/app.bsky.graph.follow/abc"
	err := dbs.Users.UpsertFollow(ctx, userDID, targetDID, uri, "cid")
	assert.NilError(t, err)

	err = dbs.Users.DeleteFollowByURI(ctx, uri)
	assert.NilError(t, err)

	following, err := dbs.Users.IsFollowing(ctx, userDID, targetDID)
	assert.NilError(t, err)
	assert.Equal(t, following, false)
}

func TestListFollows(t *testing.T) {
	ctx := context.Background()
	dbs := setupTestDB(t)
	userDID, _ := seedFollowData(t, ctx, dbs)

	target2 := "did:test:followed2"
	_, err := dbs.DB().ExecContext(ctx, `INSERT INTO users (did) VALUES (?)`, target2)
	assert.NilError(t, err)

	err = dbs.Users.UpsertFollow(ctx, userDID, "did:test:followed", "uri1", "cid1")
	assert.NilError(t, err)
	err = dbs.Users.UpsertFollow(ctx, userDID, target2, "uri2", "cid2")
	assert.NilError(t, err)

	follows, err := dbs.Users.ListFollows(ctx, userDID, 10, 0)
	assert.NilError(t, err)
	assert.Equal(t, len(follows), 2)
}

func TestListFollowers(t *testing.T) {
	ctx := context.Background()
	dbs := setupTestDB(t)
	_, targetDID := seedFollowData(t, ctx, dbs)

	follower2 := "did:test:follower2"
	_, err := dbs.DB().ExecContext(ctx, `INSERT INTO users (did) VALUES (?)`, follower2)
	assert.NilError(t, err)

	err = dbs.Users.UpsertFollow(ctx, "did:test:follower", targetDID, "uri1", "cid1")
	assert.NilError(t, err)
	err = dbs.Users.UpsertFollow(ctx, follower2, targetDID, "uri2", "cid2")
	assert.NilError(t, err)

	followers, err := dbs.Users.ListFollowers(ctx, targetDID, 10, 0)
	assert.NilError(t, err)
	assert.Equal(t, len(followers), 2)
}

func TestGetFollowDIDs(t *testing.T) {
	ctx := context.Background()
	dbs := setupTestDB(t)
	userDID, _ := seedFollowData(t, ctx, dbs)

	target2 := "did:test:followed2"
	_, err := dbs.DB().ExecContext(ctx, `INSERT INTO users (did) VALUES (?)`, target2)
	assert.NilError(t, err)

	err = dbs.Users.UpsertFollow(ctx, userDID, "did:test:followed", "uri1", "cid1")
	assert.NilError(t, err)
	err = dbs.Users.UpsertFollow(ctx, userDID, target2, "uri2", "cid2")
	assert.NilError(t, err)

	dids, err := dbs.Users.GetFollowDIDs(ctx, userDID)
	assert.NilError(t, err)
	assert.Equal(t, len(dids), 2)
}

func TestSyncFollows_AddsNewRemovesStale(t *testing.T) {
	ctx := context.Background()
	dbs := setupTestDB(t)
	userDID, _ := seedFollowData(t, ctx, dbs)

	err := dbs.Users.UpsertFollow(ctx, userDID, "did:test:old", "old-uri", "old-cid")
	assert.NilError(t, err)

	activeFollows := map[string]Follow{
		"did:test:new1": {URI: NullStr("uri1"), CID: NullStr("cid1")},
		"did:test:new2": {URI: NullStr("uri2"), CID: NullStr("cid2")},
	}

	err = dbs.Users.SyncFollows(ctx, userDID, activeFollows)
	assert.NilError(t, err)

	stillFollowing, err := dbs.Users.IsFollowing(ctx, userDID, "did:test:old")
	assert.NilError(t, err)
	assert.Equal(t, stillFollowing, false)

	following1, err := dbs.Users.IsFollowing(ctx, userDID, "did:test:new1")
	assert.NilError(t, err)
	assert.Equal(t, following1, true)

	following2, err := dbs.Users.IsFollowing(ctx, userDID, "did:test:new2")
	assert.NilError(t, err)
	assert.Equal(t, following2, true)

	dids, err := dbs.Users.GetFollowDIDs(ctx, userDID)
	assert.NilError(t, err)
	assert.Equal(t, len(dids), 2)
}

func TestUpsertFollow_Idempotent(t *testing.T) {
	ctx := context.Background()
	dbs := setupTestDB(t)
	userDID, targetDID := seedFollowData(t, ctx, dbs)

	err := dbs.Users.UpsertFollow(ctx, userDID, targetDID, "uri1", "cid1")
	assert.NilError(t, err)
	err = dbs.Users.UpsertFollow(ctx, userDID, targetDID, "uri2", "cid2")
	assert.NilError(t, err)

	follows, err := dbs.Users.ListFollows(ctx, userDID, 10, 0)
	assert.NilError(t, err)
	assert.Equal(t, len(follows), 1)
	assert.Equal(t, follows[0].URI.String, "uri2")
}
