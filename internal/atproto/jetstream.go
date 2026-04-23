package atproto

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	jsc "github.com/bluesky-social/jetstream/pkg/client"
	"github.com/bluesky-social/jetstream/pkg/models"
	"go.uber.org/atomic"

	"pkg.rbrt.fr/glean/internal/metrics"
)

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
	handler EventHandler
	logger  *slog.Logger
	cursor  atomic.Int64
}

func (s *jetstreamScheduler) AddWork(ctx context.Context, _ string, evt *models.Event) error {
	if evt.TimeUS > 0 {
		s.cursor.Store(evt.TimeUS)
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
	client *jsc.Client
	logger *slog.Logger
	sched  *jetstreamScheduler
}

func NewJetstreamConsumer(jetstreamURL string, handler EventHandler, logger *slog.Logger) *JetstreamConsumer {
	sched := &jetstreamScheduler{
		handler: handler,
		logger:  logger,
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
		},
	}

	c, err := jsc.NewClient(config, logger, sched)
	if err != nil {
		logger.Error("failed to create jetstream client", "error", err)
		return nil
	}

	return &JetstreamConsumer{
		client: c,
		logger: logger,
		sched:  sched,
	}
}

func (jc *JetstreamConsumer) Start(ctx context.Context) error {
	for {
		cursor := jc.sched.cursor.Load()
		var cursorPtr *int64
		if cursor > 0 {
			adjusted := cursor - int64(5*time.Second/time.Microsecond)
			cursorPtr = &adjusted
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
