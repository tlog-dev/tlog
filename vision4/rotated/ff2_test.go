package rotated

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func BenchmarkWrite(b *testing.B) {
	f, err := os.Create("tmpfile")
	require.NoError(b, err)

	defer f.Close()

	buf := make([]byte, 100)

	for i := 0; i < b.N; i++ {
		_, err = f.Write(buf)
		if err != nil {
			break
		}
	}

	assert.NoError(b, err)
}

func BenchmarkStatWrite(b *testing.B) {
	f, err := os.Create("tmpfile")
	require.NoError(b, err)

	defer f.Close()

	buf := make([]byte, 100)

	for i := 0; i < b.N; i++ {
		_, err = f.Write(buf)
		if err != nil {
			break
		}
	}

	assert.NoError(b, err)
}
