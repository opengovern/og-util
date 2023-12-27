package fp_test

import (
	"testing"

	"github.com/kaytu-io/kaytu-util/pkg/fp"
	"github.com/stretchr/testify/require"
)

func TestIncludesWithStrings(t *testing.T) {
	require := require.New(t)

	require.True(fp.Includes("Parham", []string{"Parham", "Perham"}))
	require.True(fp.Includes("Perham", []string{"Parham", "Perham"}))
	require.False(fp.Includes("Hassan", []string{"Parham", "Perham"}))
}

func TestIncludesWithNumbers(t *testing.T) {
	require := require.New(t)

	require.True(fp.Includes(1373, []int{1378, 1373}))
	require.True(fp.Includes(1378, []int{1378, 1373}))
	require.False(fp.Includes(1372, []int{1378, 1373}))
}
