package fp_test

import (
	"testing"

	"github.com/opengovern/og-util/pkg/fp"
	"github.com/stretchr/testify/require"
)

func TestFromMap(t *testing.T) {
	require := require.New(t)

	type student struct {
		Name string `json:"name,omitempty"`
		ID   int    `json:"id,omitempty"`
	}

	input := map[string]any{
		"name": "Parham Alvani",
		"id":   9231058,
	}

	s, err := fp.FromMap[student](input)
	require.NoError(err)
	require.Equal("Parham Alvani", s.Name)
	require.Equal(9231058, s.ID)
}
