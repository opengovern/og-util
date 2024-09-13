package httpserver

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/kaytu-io/kaytu-util/pkg/metrics"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"gopkg.in/go-playground/validator.v9"

	"go.uber.org/zap"
)

var (
	agentHost   = os.Getenv("JAEGER_AGENT_HOST")
	serviceName = os.Getenv("JAEGER_SERVICE_NAME")
	sampleRate  = os.Getenv("JAEGER_SAMPLE_RATE")
)

type Routes interface {
	Register(router *echo.Echo)
}

type EmptyRoutes struct{}

func (EmptyRoutes) Register(router *echo.Echo) {}

func Register(logger *zap.Logger, routes Routes) (*echo.Echo, *sdktrace.TracerProvider) {
	e := echo.New()
	e.HideBanner = true

	e.Use(middleware.Recover())
	e.Use(Logger(logger))
	e.Use(middleware.GzipWithConfig(middleware.GzipConfig{
		Skipper: func(c echo.Context) bool {
			// skip metric endpoints
			if strings.HasPrefix(c.Path(), "/metrics") {
				return true
			}
			// skip if client does not accept gzip
			acceptEncodingHeader := c.Request().Header.Values(echo.HeaderAcceptEncoding)
			for _, value := range acceptEncodingHeader {
				if strings.TrimSpace(value) == "gzip" {
					return false
				}
			}
			return true
		},
		Level: 5,
	}))

	metrics.AddEchoMiddleware(e)

	e.Pre(middleware.RemoveTrailingSlash())

	tp, err := initTracer()
	if err != nil {
		logger.Error(err.Error())
		return nil, nil
	}

	e.Validator = customValidator{
		validate: validator.New(),
	}

	routes.Register(e)

	return e, tp
}

func RegisterAndStart(ctx context.Context, logger *zap.Logger, address string, routes Routes) error {
	e, tp := Register(logger, routes)

	defer func() {
		if err := tp.Shutdown(ctx); err != nil {
		}
	}()
	e.Use(otelecho.Middleware(serviceName))

	return e.Start(address)
}

type customValidator struct {
	validate *validator.Validate
}

func (v customValidator) Validate(i interface{}) error {
	return v.validate.Struct(i)
}

func QueryArrayParam(ctx echo.Context, paramName string) []string {
	var values []string
	for k, v := range ctx.QueryParams() {
		if k == paramName || k == paramName+"[]" {
			values = append(values, v...)
		}
	}
	return values
}

func QueryMapParam(ctx echo.Context, paramName string) map[string][]string {
	mapParam := make(map[string][]string)
	for key, values := range ctx.QueryParams() {
		if strings.HasPrefix(key, fmt.Sprintf("%s[", paramName)) && strings.HasSuffix(key, "]") {
			tagKey := key[len(fmt.Sprintf("%s[", paramName)) : len(key)-1]
			mapParam[tagKey] = values
		}
	}
	return mapParam
}

func initTracer() (*sdktrace.TracerProvider, error) {
	exporter, err := jaeger.New(jaeger.WithAgentEndpoint(jaeger.WithAgentHost(agentHost)))
	if err != nil {
		return nil, err
	}

	sampleRateFloat := 1.0
	if sampleRate != "" {
		sampleRateFloat, err = strconv.ParseFloat(sampleRate, 64)
		if err != nil {
			fmt.Println("Error parsing sample rate for Jaeger. Using default value of 1.0", err)
			sampleRateFloat = 1
		}
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.TraceIDRatioBased(sampleRateFloat)),
		sdktrace.WithBatcher(exporter),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	return tp, nil
}
