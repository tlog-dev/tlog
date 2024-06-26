package tlslog

import (
	"bytes"
	"errors"
	"io"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"golang.org/x/exp/slog"

	"tlog.app/go/tlog"
	"tlog.app/go/tlog/convert"
	"tlog.app/go/tlog/tlwire"
)

func TestSlog(t *testing.T) {
	var b bytes.Buffer
	d := tlwire.NewDumper(os.Stderr)
	w := io.MultiWriter(d, &b)
	l := tlog.New(w)
	h := Wrap(l)
	s := slog.New(h)

	s.Info("message", "a", "b", "c", 4)

	ss := s.With("a", "b", "c", 5)
	ss.Warn("warning", "d", errors.New("holy error"))

	ss = s.With("a", "b").WithGroup("group").With("c", 6).WithGroup("group2").With("d", "e")

	ss.Info("go deeper", slog.Group("gr", slog.Duration("d", time.Second)))

	s.Info("corner cases", "", "b", slog.Group("", slog.String("embedded", "value"), slog.Group("omitted")))

	//

	var bb bytes.Buffer

	lfw := convert.NewLogfmt(&bb)
	rew := convert.NewRewriter(lfw)
	rew.Rule = convert.NewKeyRenamer(nil, convert.RenameRule{
		Path: []tlog.RawMessage{
			rew.AppendString(nil, tlog.KeyTimestamp),
			tlog.RawTag(tlwire.Semantic, tlwire.Time),
		},
		Remove: true,
	}, convert.RenameRule{
		Path: []tlog.RawMessage{
			rew.AppendString(nil, tlog.KeyCaller),
			tlog.RawTag(tlwire.Semantic, tlwire.Caller),
		},
		Remove: true,
	}, convert.RenameRule{
		Path: []tlog.RawMessage{
			rew.AppendString(nil, tlog.KeyLogLevel),
			tlog.RawTag(tlwire.Semantic, tlog.WireLogLevel),
		},
		Rename: []byte("level"),
	})

	_, err := convert.Copy(rew, &b)
	assert.NoError(t, err)
	assert.Equal(t, `
_m=message  a=b  c=4
_m=warning  level=1  a=b  c=5  d="holy error"
_m="go deeper"  a=b  group.c=6  group.group2.d=e  group.group2.gr.d=1s
_m="corner cases"  embedded=value
`, "\n"+bb.String())
}
