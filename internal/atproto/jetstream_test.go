package atproto

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"gotest.tools/v3/assert"
)

func testConsumer(t *testing.T, provider KnownDIDsProvider) *JetstreamConsumer {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewJetstreamConsumer("wss://jetstream.example", func(context.Context, *Event) error { return nil }, logger, nil, provider)
}

func TestRefreshWantedDIDs_UsesProvider(t *testing.T) {
	ctx := context.Background()
	jc := testConsumer(t, func(context.Context) ([]string, error) {
		return []string{"did:test:a", "did:test:b"}, nil
	})

	assert.NilError(t, jc.refreshWantedDIDs(ctx))
	assert.DeepEqual(t, []string{"did:test:a", "did:test:b"}, jc.cfg.WantedDids)
}

func TestRefreshWantedDIDs_ErrorKeepsPreviousFilter(t *testing.T) {
	ctx := context.Background()
	first := true
	jc := testConsumer(t, func(context.Context) ([]string, error) {
		if first {
			first = false
			return []string{"did:test:a"}, nil
		}
		return nil, errors.New("db down")
	})

	assert.NilError(t, jc.refreshWantedDIDs(ctx))
	assert.Equal(t, 1, len(jc.cfg.WantedDids))

	assert.ErrorContains(t, jc.refreshWantedDIDs(ctx), "db down")
	assert.Equal(t, 1, len(jc.cfg.WantedDids), "previous filter must survive provider failure")
}

func TestRefreshWantedDIDs_EmptyListIsNotAnError(t *testing.T) {
	ctx := context.Background()
	jc := testConsumer(t, func(context.Context) ([]string, error) { return nil, nil })

	assert.NilError(t, jc.refreshWantedDIDs(ctx))
	assert.Equal(t, 0, len(jc.cfg.WantedDids))
}

func TestRefreshWantedDIDs_NoProviderIsNoop(t *testing.T) {
	ctx := context.Background()
	jc := testConsumer(t, nil)

	assert.NilError(t, jc.refreshWantedDIDs(ctx))
	assert.Equal(t, 0, len(jc.cfg.WantedDids))
}
