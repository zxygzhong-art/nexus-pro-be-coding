package telemetry

import (
	"context"
	"net/url"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.41.0"
)

// Config 定義組態的資料結構。
type Config struct {
	Enabled  bool
	Service  string
	Endpoint string
	Insecure bool
	Env      string
}

// Init 處理 init。
func Init(ctx context.Context, cfg Config) (func(context.Context) error, error) {
	if !cfg.Enabled || cfg.Endpoint == "" {
		return func(context.Context) error { return nil }, nil
	}
	serviceName := cfg.Service
	if serviceName == "" {
		serviceName = "nexus-pro-api"
	}
	endpoint := cfg.Endpoint
	if strings.Contains(endpoint, "://") {
		parsed, err := url.Parse(endpoint)
		if err != nil {
			return nil, err
		}
		if parsed.Host != "" {
			endpoint = parsed.Host
		}
	}
	options := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(endpoint),
	}
	if cfg.Insecure {
		options = append(options, otlptracegrpc.WithInsecure())
	}
	exporter, err := otlptracegrpc.New(ctx, options...)
	if err != nil {
		return nil, err
	}
	res, err := resource.Merge(resource.Default(), resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(serviceName),
		semconv.DeploymentEnvironmentNameKey.String(cfg.Env),
		attribute.String("service.namespace", "nexus-pro"),
	))
	if err != nil {
		return nil, err
	}
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	return provider.Shutdown, nil
}
