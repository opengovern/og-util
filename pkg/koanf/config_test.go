package koanf_test

import (
	"os"
	"testing"

	"github.com/opengovern/og-util/pkg/koanf"
	"github.com/stretchr/testify/require"
)

type Config struct {
	Postgres koanf.Postgres `koanf:"postgres"`
}

func TestProvideUsingDefault(t *testing.T) {
	require := require.New(t)

	cfg := koanf.Provide("testing", Config{
		Postgres: koanf.Postgres{
			Host:     "psql.io",
			Username: "admin",
			Password: "admin",
		},
	})

	require.Equal("psql.io", cfg.Postgres.Host)
	require.Equal("admin", cfg.Postgres.Password)
}

func TestProvideUsingEnv(t *testing.T) {
	os.Setenv("TESTING_POSTGRES__HOST", "psql.com")
	defer os.Setenv("TESTING_POSTGRES__HOST", "")

	require := require.New(t)

	cfg := koanf.Provide("testing", Config{
		Postgres: koanf.Postgres{
			Host:     "psql.io",
			Username: "admin",
			Password: "admin",
		},
	})

	require.Equal("psql.com", cfg.Postgres.Host)
	require.Equal("admin", cfg.Postgres.Password)
}
