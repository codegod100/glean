package atproto

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	jsc "github.com/bluesky-social/jetstream/pkg/client"
	"github.com/bluesky-social/jetstream/pkg/models"

	"pkg.rbrt.fr/glean/internal/metrics"
)

// CursorStore stores the Jetstream cursor.
type CursorStore interface {
	LoadCursor(ctx context.Context) (*int64, error)
	SaveCursor(ctx context.Context, cursor int64) error
}

type Event struct {
	Type       string
	DID        string
	Collection string
	RKey       string
	URI        string
	CID        string
	Value      json.RawMessage
}

type EventHandler func(ctx context.Context, event *Event) error

type jetstreamScheduler struct {
	handler     EventHandler
	logger      *slog.Logger
	cursorStore *BatchCursorStore
}

func (s *jetstreamScheduler) AddWork(ctx context.Context, _ string, evt *models.Event) error {
	if evt.TimeUS > 0 {
		s.cursorStore.Record(ctx, evt.TimeUS)
	}

	if evt.Kind != models.EventKindCommit || evt.Commit == nil {
		return nil
	}

	c := evt.Commit
	if c.Operation != models.CommitOperationCreate &&
		c.Operation != models.CommitOperationUpdate &&
		c.Operation != models.CommitOperationDelete {
		return nil
	}

	e := &Event{
		Type:       c.Operation,
		DID:        evt.Did,
		Collection: c.Collection,
		RKey:       c.RKey,
		URI:        fmt.Sprintf("at://%s/%s/%s", evt.Did, c.Collection, c.RKey),
		CID:        c.CID,
		Value:      c.Record,
	}

	if err := s.handler(ctx, e); err != nil {
		s.logger.Error("jetstream handler error", "error", err)
		metrics.JetstreamErrors.Inc()
	}

	metrics.JetstreamEvents.WithLabelValues(c.Collection, c.Operation).Inc()
	return nil
}

func (s *jetstreamScheduler) Shutdown() {}

type JetstreamConsumer struct {
	client      *jsc.Client
	logger      *slog.Logger
	sched       *jetstreamScheduler
	cursorStore *BatchCursorStore
	rewind      time.Duration
}

func NewJetstreamConsumer(jetstreamURL string, handler EventHandler, logger *slog.Logger, cursorStore CursorStore) *JetstreamConsumer {
	batchCursor := NewBatchCursorStore(cursorStore, 5*time.Second, logger)

	sched := &jetstreamScheduler{
		handler:     handler,
		logger:      logger,
		cursorStore: batchCursor,
	}

	wsURL := jetstreamURL
	if !strings.HasSuffix(wsURL, "/subscribe") {
		wsURL += "/subscribe"
	}

	config := &jsc.ClientConfig{
		Compress:     true,
		WebsocketURL: wsURL,
		ExtraHeaders: map[string]string{
			"User-Agent": "glean/1.0",
		},
		WantedCollections: []string{
			CollectionSubscription,
			CollectionAnnotation,
			CollectionLike,
			CollectionBskyFollow,
			CollectionTangledFollow,
			CollectionMarginNote,
			CollectionStandardPublication,
			CollectionStandardDocument,
		},
	}

	c, err := jsc.NewClient(config, logger, sched)
	if err != nil {
		logger.Error("failed to create jetstream client", "error", err)
		return nil
	}

	rewind := 5 * time.Second
	if cursorStore == nil {
		rewind = 0
	}

	return &JetstreamConsumer{
		client:      c,
		logger:      logger,
		sched:       sched,
		cursorStore: batchCursor,
		rewind:      rewind,
	}
}

func (jc *JetstreamConsumer) Start(ctx context.Context) error {
	go jc.cursorStore.Run(ctx)

	for {
		var cursorPtr *int64
		if jc.cursorStore != nil {
			cur, err := jc.cursorStore.LoadCursor(ctx)
			if err != nil {
				jc.logger.Warn("failed to load cursor, starting from now", "error", err)
			} else if cur != nil {
				rewound := max(*cur-int64(jc.rewind/time.Microsecond), 0)
				cursorPtr = &rewound
				jc.logger.Info("resuming jetstream", "cursor_us", *cur, "rewound_us", rewound)
			}
		}

		err := jc.client.ConnectAndRead(ctx, cursorPtr)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			jc.logger.Warn("jetstream connection lost", "error", err)
			metrics.JetstreamReconnects.Inc()
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
}

type BatchCursorStore struct {
	store  CursorStore
	mu     sync.Mutex
	cursor int64
	dirty  bool
	flush  time.Duration
	logger *slog.Logger
}

func NewBatchCursorStore(store CursorStore, flushInterval time.Duration, logger *slog.Logger) *BatchCursorStore {
	return &BatchCursorStore{store: store, flush: flushInterval, logger: logger}
}

func (b *BatchCursorStore) LoadCursor(ctx context.Context) (*int64, error) {
	return b.store.LoadCursor(ctx)
}

func (b *BatchCursorStore) Record(_ context.Context, cursor int64) {
	b.mu.Lock()
	b.cursor = cursor
	b.dirty = true
	b.mu.Unlock()
}

func (b *BatchCursorStore) Run(ctx context.Context) {
	ticker := time.NewTicker(b.flush)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			b.flushNow(context.Background())
			return
		case <-ticker.C:
			b.flushNow(ctx)
		}
	}
}

func (b *BatchCursorStore) flushNow(ctx context.Context) {
	b.mu.Lock()
	if !b.dirty {
		b.mu.Unlock()
		return
	}
	cursor := b.cursor
	b.dirty = false
	b.mu.Unlock()

	if err := b.store.SaveCursor(ctx, cursor); err != nil {
		b.logger.Warn("failed to save cursor", "error", err)
	}
}
