package observability

import (
	"context"
	"time"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	tracerProvider *sdktrace.TracerProvider
)

// InitTracer initializes the OpenTelemetry tracer
func InitTracer() {
	if !config.AppConfig.TracingEnabled {
		logging.Logger.Info("tracing is disabled")
		return
	}

	ctx := context.Background()

	// Create OTLP exporter
	client := otlptracegrpc.NewClient(
		otlptracegrpc.WithInsecure(),
		otlptracegrpc.WithEndpoint(config.AppConfig.TracingEndpoint),
		otlptracegrpc.WithDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
	)
	exporter, err := otlptrace.New(ctx, client)
	if err != nil {
		logging.Logger.Error("failed to create OTLP exporter", zap.Error(err))
		return
	}

	// Create resource
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String("app-rmi"),
			semconv.ServiceVersionKey.String("v1.0.0"),
		),
	)
	if err != nil {
		logging.Logger.Error("failed to create resource", zap.Error(err))
		return
	}

	// Create trace provider
	tracerProvider = sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter,
			sdktrace.WithMaxExportBatchSize(512),
			sdktrace.WithBatchTimeout(time.Second*10),
			sdktrace.WithMaxQueueSize(2048),
		),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)

	// Set global trace provider
	otel.SetTracerProvider(tracerProvider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	logging.Logger.Info("tracer initialized")
}

// ShutdownTracer shuts down the tracer provider
func ShutdownTracer() {
	if tracerProvider == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	if err := tracerProvider.Shutdown(ctx); err != nil {
		logging.Logger.Error("failed to shutdown tracer provider", zap.Error(err))
	}
}
