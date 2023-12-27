package koanf_test

import (
	"os"
	"testing"

	"github.com/kaytu-io/kaytu-util/pkg/koanf"
	"github.com/stretchr/testify/require"
)

type Config struct {
	RabbitMQ koanf.RabbitMQ `koanf:"rabbitmq"`
}

func TestProvideUsingDefault(t *testing.T) {
	require := require.New(t)

	cfg := koanf.Provide("testing", Config{
		RabbitMQ: koanf.RabbitMQ{
			Service:  "rabbitmq.io",
			Username: "admin",
			Password: "admin",
		},
	})

	require.Equal("rabbitmq.io", cfg.RabbitMQ.Service)
	require.Equal("admin", cfg.RabbitMQ.Password)
}

func TestProvideUsingEnv(t *testing.T) {
	os.Setenv("TESTING_RABBITMQ__SERVICE", "rabbitmq.com")
	defer os.Setenv("TESTING_RABBITMQ__SERVICE", "")

	require := require.New(t)

	cfg := koanf.Provide("testing", Config{
		RabbitMQ: koanf.RabbitMQ{
			Service:  "rabbitmq.io",
			Username: "admin",
			Password: "admin",
		},
	})

	require.Equal("rabbitmq.com", cfg.RabbitMQ.Service)
	require.Equal("admin", cfg.RabbitMQ.Password)
}
