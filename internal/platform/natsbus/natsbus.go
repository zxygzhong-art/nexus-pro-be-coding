package natsbus

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	"nexus-pro-be/internal/domain"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const (
	DefaultURL            = "nats://127.0.0.1:24222"
	DefaultStream         = "NEXUS_EVENTS"
	DefaultConsumerPrefix = "nexus"
	EventsSubjectPattern  = "events.>"

	HeaderTenantID  = "Nexus-Tenant-Id"
	HeaderEventID   = "Nexus-Event-Id"
	HeaderEventType = "Nexus-Event-Type"
)

// Config defines the NATS JetStream client configuration.
type Config struct {
	URL    string
	Stream string
}

// EventPublisher publishes canonical domain event envelopes.
type EventPublisher interface {
	Publish(context.Context, string, domain.DomainEventEnvelope) error
}

// EventSubscriber subscribes to a durable JetStream consumer.
type EventSubscriber interface {
	Subscribe(context.Context, SubscriptionOptions, MessageHandler) (Subscription, error)
}

// SubscriptionOptions defines durable consumer subscription settings.
type SubscriptionOptions struct {
	Stream        string
	Durable       string
	FilterSubject string
	MaxDeliver    int
	AckWait       time.Duration
	MaxAckPending int
}

// MessageHandler handles one event message. The handler owns ack/nak.
type MessageHandler func(context.Context, Message) error

// Message is the small message surface jobs need from JetStream.
type Message interface {
	Subject() string
	Header(string) string
	Data() []byte
	Metadata() MessageMetadata
	Ack() error
	Nak() error
}

// MessageMetadata exposes delivery metadata needed for retry/dead-letter handling.
type MessageMetadata struct {
	NumDelivered uint64
}

// Subscription represents an active event subscription.
type Subscription interface {
	Stop()
	Drain()
	Closed() <-chan struct{}
}

// Client wraps a NATS connection and JetStream context.
type Client struct {
	conn   *nats.Conn
	js     jetstream.JetStream
	stream string
	logger *slog.Logger
}

// NormalizeConfig applies platform defaults.
func NormalizeConfig(cfg Config) Config {
	cfg.URL = strings.TrimSpace(cfg.URL)
	if cfg.URL == "" {
		cfg.URL = DefaultURL
	}
	cfg.Stream = strings.TrimSpace(cfg.Stream)
	if cfg.Stream == "" {
		cfg.Stream = DefaultStream
	}
	return cfg
}

// Connect opens a NATS connection, creates a JetStream context, and ensures the event stream exists.
func Connect(ctx context.Context, cfg Config, logger *slog.Logger) (*Client, error) {
	cfg = NormalizeConfig(cfg)
	if logger == nil {
		logger = slog.Default()
	}
	nc, err := nats.Connect(
		cfg.URL,
		nats.Name("nexus-pro-be"),
		nats.Timeout(5*time.Second),
	)
	if err != nil {
		return nil, err
	}
	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return nil, err
	}
	client := &Client{conn: nc, js: js, stream: cfg.Stream, logger: logger}
	if err := client.EnsureStream(ctx); err != nil {
		nc.Close()
		return nil, err
	}
	return client, nil
}

// EnsureStream creates or updates the single platform events stream.
func (c *Client) EnsureStream(ctx context.Context) error {
	if c == nil || c.js == nil {
		return errors.New("natsbus client is not configured")
	}
	_, err := c.js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:        c.stream,
		Description: "Nexus platform domain events",
		Subjects:    []string{EventsSubjectPattern},
		Retention:   jetstream.LimitsPolicy,
		Storage:     jetstream.FileStorage,
		Replicas:    1,
		Duplicates:  24 * time.Hour,
	})
	return err
}

// Publish writes an event envelope to JetStream and waits for the server ack.
func (c *Client) Publish(ctx context.Context, subject string, envelope domain.DomainEventEnvelope) error {
	stream := ""
	if c != nil {
		stream = c.stream
	}
	ctx, span := otel.Tracer("nexus-pro-be/internal/platform/natsbus").Start(ctx, "natsbus.publish",
		trace.WithSpanKind(trace.SpanKindProducer),
		trace.WithAttributes(
			attribute.String("messaging.system", "nats"),
			attribute.String("messaging.destination.name", subject),
			attribute.String("messaging.nats.stream", stream),
			attribute.String("tenant_id", envelope.TenantID),
			attribute.String("event_id", envelope.EventID),
			attribute.String("event_type", envelope.EventType),
		),
	)
	defer finishSpan(span, nil)

	if c == nil || c.js == nil {
		err := errors.New("natsbus publisher is not configured")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	raw, err := json.Marshal(envelope)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	msg := &nats.Msg{
		Subject: subject,
		Header:  nats.Header{},
		Data:    raw,
	}
	msg.Header.Set(HeaderTenantID, envelope.TenantID)
	msg.Header.Set(HeaderEventID, envelope.EventID)
	msg.Header.Set(HeaderEventType, envelope.EventType)
	_, err = c.js.PublishMsg(ctx, msg, jetstream.WithMsgID(envelope.EventID), jetstream.WithExpectStream(c.stream))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

// Subscribe creates or updates a durable pull consumer and starts callback consumption.
func (c *Client) Subscribe(ctx context.Context, opts SubscriptionOptions, handler MessageHandler) (Subscription, error) {
	if c == nil || c.js == nil {
		return nil, errors.New("natsbus subscriber is not configured")
	}
	if handler == nil {
		return nil, errors.New("natsbus subscriber requires handler")
	}
	opts = c.normalizeSubscriptionOptions(opts)
	consumer, err := c.js.CreateOrUpdateConsumer(ctx, opts.Stream, jetstream.ConsumerConfig{
		Name:          opts.Durable,
		Durable:       opts.Durable,
		Description:   "Nexus platform event consumer " + opts.Durable,
		DeliverPolicy: jetstream.DeliverAllPolicy,
		AckPolicy:     jetstream.AckExplicitPolicy,
		AckWait:       opts.AckWait,
		MaxDeliver:    opts.MaxDeliver,
		FilterSubject: opts.FilterSubject,
		MaxAckPending: opts.MaxAckPending,
	})
	if err != nil {
		return nil, err
	}
	consumeCtx, err := consumer.Consume(func(msg jetstream.Msg) {
		ctx, span := otel.Tracer("nexus-pro-be/internal/platform/natsbus").Start(ctx, "natsbus.consume",
			trace.WithSpanKind(trace.SpanKindConsumer),
			trace.WithAttributes(
				attribute.String("messaging.system", "nats"),
				attribute.String("messaging.destination.name", msg.Subject()),
				attribute.String("messaging.nats.stream", opts.Stream),
				attribute.String("event_id", msg.Headers().Get(HeaderEventID)),
				attribute.String("event_type", msg.Headers().Get(HeaderEventType)),
				attribute.String("tenant_id", msg.Headers().Get(HeaderTenantID)),
			),
		)
		err := handler(ctx, jetStreamMessage{msg: msg})
		finishSpan(span, err)
		if err != nil && c.logger != nil {
			c.logger.WarnContext(ctx, "nats event handler failed", "subject", msg.Subject(), "error", err)
		}
	}, jetstream.PullMaxMessages(100), jetstream.ConsumeErrHandler(func(_ jetstream.ConsumeContext, err error) {
		if c.logger != nil {
			c.logger.WarnContext(ctx, "nats consumer error", "durable", opts.Durable, "error", err)
		}
	}))
	if err != nil {
		return nil, err
	}
	return jetStreamSubscription{ctx: consumeCtx}, nil
}

// Ping verifies JetStream is available for readiness checks.
func (c *Client) Ping(ctx context.Context) error {
	if c == nil || c.js == nil {
		return errors.New("natsbus client is not configured")
	}
	_, err := c.js.AccountInfo(ctx)
	return err
}

// Close closes the underlying NATS connection.
func (c *Client) Close() error {
	if c != nil && c.conn != nil {
		c.conn.Close()
	}
	return nil
}

func (c *Client) normalizeSubscriptionOptions(opts SubscriptionOptions) SubscriptionOptions {
	opts.Stream = strings.TrimSpace(opts.Stream)
	if opts.Stream == "" {
		opts.Stream = c.stream
	}
	opts.Durable = strings.TrimSpace(opts.Durable)
	opts.FilterSubject = strings.TrimSpace(opts.FilterSubject)
	if opts.MaxDeliver <= 0 {
		opts.MaxDeliver = 5
	}
	if opts.AckWait <= 0 {
		opts.AckWait = 30 * time.Second
	}
	if opts.MaxAckPending <= 0 {
		opts.MaxAckPending = 100
	}
	return opts
}

type jetStreamMessage struct {
	msg jetstream.Msg
}

func (m jetStreamMessage) Subject() string {
	return m.msg.Subject()
}

func (m jetStreamMessage) Header(key string) string {
	return m.msg.Headers().Get(key)
}

func (m jetStreamMessage) Data() []byte {
	return m.msg.Data()
}

func (m jetStreamMessage) Metadata() MessageMetadata {
	meta, err := m.msg.Metadata()
	if err != nil || meta == nil {
		return MessageMetadata{}
	}
	return MessageMetadata{NumDelivered: meta.NumDelivered}
}

func (m jetStreamMessage) Ack() error {
	return m.msg.Ack()
}

func (m jetStreamMessage) Nak() error {
	return m.msg.Nak()
}

type jetStreamSubscription struct {
	ctx jetstream.ConsumeContext
}

func (s jetStreamSubscription) Stop() {
	s.ctx.Stop()
}

func (s jetStreamSubscription) Drain() {
	s.ctx.Drain()
}

func (s jetStreamSubscription) Closed() <-chan struct{} {
	return s.ctx.Closed()
}

func finishSpan(span trace.Span, err error) {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	span.End()
}
