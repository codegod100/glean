package atproto

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/gorilla/websocket"
)

type FirehoseEvent struct {
	Type       string
	DID        string
	Collection string
	RKey       string
	URI        string
	CID        string
	Value      json.RawMessage
}

type FirehoseHandler func(ctx context.Context, event *FirehoseEvent) error

type FirehoseConsumer struct {
	relayURL    string
	handler     FirehoseHandler
	cursor      int64
	logger      *slog.Logger
	collections map[string]bool
}

func NewFirehoseConsumer(relayURL string, handler FirehoseHandler, logger *slog.Logger) *FirehoseConsumer {
	return &FirehoseConsumer{
		relayURL: relayURL,
		handler:  handler,
		logger:   logger,
		collections: map[string]bool{
			"at.glean.subscription": true,
			"at.glean.annotation":   true,
			"at.glean.like":         true,
		},
	}
}

func (fc *FirehoseConsumer) Start(ctx context.Context) error {
	for {
		err := fc.connect(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			fc.logger.Error("firehose connection error", "error", err)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
}

func (fc *FirehoseConsumer) connect(ctx context.Context) error {
	u, err := url.Parse(fc.relayURL)
	if err != nil {
		return fmt.Errorf("parsing relay URL: %w", err)
	}

	scheme := "wss"
	if u.Scheme == "http" || u.Scheme == "ws" {
		scheme = "ws"
	}

	wsURL := fmt.Sprintf("%s://%s/xrpc/com.atproto.sync.subscribeRepos", scheme, u.Host)
	if fc.cursor > 0 {
		wsURL += fmt.Sprintf("?cursor=%d", fc.cursor)
	}

	fc.logger.Info("connecting to firehose", "url", wsURL)

	conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, http.Header{})
	if err != nil {
		return fmt.Errorf("dialing firehose: %w", err)
	}
	defer conn.Close()

	fc.logger.Info("firehose connected")

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		_, msg, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("reading firehose: %w", err)
		}

		fc.handleMessage(ctx, msg)
	}
}

func (fc *FirehoseConsumer) handleMessage(ctx context.Context, msg []byte) {
	var frame struct {
		Type   string          `json:"type"`
		Commit json.RawMessage `json:"commit"`
		Seq    int64           `json:"seq"`
	}

	if err := json.Unmarshal(msg, &frame); err != nil {
		return
	}

	if frame.Seq > 0 {
		fc.cursor = frame.Seq
	}

	if frame.Type == "#commit" {
		fc.parseCommit(ctx, frame.Commit)
	}
}

func (fc *FirehoseConsumer) parseCommit(ctx context.Context, raw json.RawMessage) {
	var commit struct {
		Did    string `json:"did"`
		Ops    []struct {
			Action string          `json:"action"`
			Path   string          `json:"path"`
			CID    json.RawMessage `json:"cid"`
			Record json.RawMessage `json:"record"`
		} `json:"ops"`
	}

	if err := json.Unmarshal(raw, &commit); err != nil {
		return
	}

	for _, op := range commit.Ops {
		parts := splitPath(op.Path)
		if len(parts) != 2 {
			continue
		}

		collection := parts[0]
		rkey := parts[1]

		if !fc.collections[collection] {
			continue
		}

		var action string
		switch op.Action {
		case "create":
			action = "create"
		case "update":
			action = "update"
		case "delete":
			action = "delete"
		default:
			continue
		}

		evt := &FirehoseEvent{
			Type:       action,
			DID:        commit.Did,
			Collection: collection,
			RKey:       rkey,
			URI:        fmt.Sprintf("at://%s/%s/%s", commit.Did, collection, rkey),
			Value:      op.Record,
		}

		if op.CID != nil {
			evt.CID = string(op.CID)
		}

		if err := fc.handler(ctx, evt); err != nil {
			fc.logger.Error("firehose handler error", "error", err)
		}
	}
}

func splitPath(p string) []string {
	if p == "" {
		return nil
	}
	var parts []string
	start := 0
	for i := 0; i < len(p); i++ {
		if p[i] == '/' {
			parts = append(parts, p[start:i])
			start = i + 1
		}
	}
	parts = append(parts, p[start:])
	return parts
}
