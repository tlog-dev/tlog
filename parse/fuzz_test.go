package parse

import (
	"bytes"
	"encoding/hex"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFuzzJSONCrashes(t *testing.T) {
	testFuzz(t, filepath.Join("JSON_wd", "crashers"))
}

func TestFuzzJSONCorpus(t *testing.T) {
	testFuzz(t, filepath.Join("JSON_wd", "corpus"))
}

func TestFuzzProtoCrashes(t *testing.T) {
	testFuzz(t, filepath.Join("Proto_wd", "crashers"))
}

func TestFuzzProtoCorpus(t *testing.T) {
	testFuzz(t, filepath.Join("Proto_wd", "corpus"))
}

func testFuzz(t *testing.T, dir string) {
	base := filepath.Join("..", "fuzz", dir)

	_, err := os.Stat(base)
	if os.IsNotExist(err) {
		t.Skip("no files")
	}
	require.NoError(t, err)

	fs, err := ioutil.ReadDir(base)
	require.NoError(t, err)

	for _, f := range fs {
		data, err := ioutil.ReadFile(filepath.Join(base, f.Name()))
		require.NoError(t, err)

		t.Run(f.Name(), func(t *testing.T) {
			r := NewJSONReader(bytes.NewReader(data))

			for {
				_, err := r.Read()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Logf("parse: %v\ndata:\n%v", err, hex.Dump(data))
					break
				}
			}
		})
	}
}
