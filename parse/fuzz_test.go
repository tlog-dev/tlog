package parse

import (
	"bytes"
	"encoding/hex"
	"flag"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/nikandfor/tlog"
	"github.com/stretchr/testify/require"
)

func TestFuzzJSONCrashes(t *testing.T) {
	testFuzz(t, filepath.Join("JSON_wd", "crashers"), func(r io.Reader) Reader { return NewJSONReader(r) })
}

func TestFuzzJSONCorpus(t *testing.T) {
	testFuzz(t, filepath.Join("JSON_wd", "corpus"), func(r io.Reader) Reader { return NewJSONReader(r) })
}

func TestFuzzProtoCrashes(t *testing.T) {
	testFuzz(t, filepath.Join("Proto_wd", "crashers"), func(r io.Reader) Reader { return NewProtoReader(r) })
}

func TestFuzzProtoCorpus(t *testing.T) {
	testFuzz(t, filepath.Join("Proto_wd", "corpus"), func(r io.Reader) Reader { return NewProtoReader(r) })
}

func testFuzz(t *testing.T, dir string, newr func(r io.Reader) Reader) {
	base := filepath.Join("..", "fuzz", dir)

	_, err := os.Stat(base)
	if os.IsNotExist(err) {
		t.Skip("no files")
	}
	require.NoError(t, err)

	fs, err := ioutil.ReadDir(base)
	require.NoError(t, err)

	for _, f := range fs {
		if strings.HasSuffix(f.Name(), ".quoted") || strings.HasSuffix(f.Name(), ".output") {
			continue
		}

		data, err := ioutil.ReadFile(filepath.Join(base, f.Name()))
		require.NoError(t, err)

		t.Run(f.Name(), func(t *testing.T) {
			var err error
			defer func() {
				p := recover()
				if err != nil || p != nil {
					t.Logf("%v\ndata:\n%v", err, hex.Dump(data))
				}
				if p == nil {
					return
				}

				s := debug.Stack()
				t.Fatalf("panic: %v\n%s", p, s)
			}()

			r := newr(bytes.NewReader(data))

			for {
				_, err = r.Read()
				if err == io.EOF {
					err = nil
					break
				}
				if err != nil {
					break
				}
			}
		})
	}
}

var (
	v = flag.String("tlog", "", "tlog verbosity filter")
)

func TestMain(m *testing.M) {
	flag.Parse()

	if *v != "" {
		tlog.DefaultLogger = tlog.New(tlog.NewConsoleWriter(os.Stderr, tlog.LdetFlags|tlog.Lfuncname))
		tlog.SetFilter(*v)
	}

	os.Exit(m.Run())
}
