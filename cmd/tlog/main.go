package main

import (
	"context"
	"debug/elf"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/gin-gonic/gin"
	"github.com/nikandfor/cli"
	"github.com/nikandfor/errors"
	"github.com/nikandfor/graceful"
	"go.etcd.io/bbolt"

	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/agent"
	"github.com/nikandfor/tlog/compress"
	"github.com/nikandfor/tlog/convert"
	"github.com/nikandfor/tlog/ext/tlbolt"
	"github.com/nikandfor/tlog/ext/tlclickhouse"
	"github.com/nikandfor/tlog/ext/tlflag"
	"github.com/nikandfor/tlog/ext/tlgin"
	"github.com/nikandfor/tlog/processor"
	"github.com/nikandfor/tlog/rotated"
	"github.com/nikandfor/tlog/tlio"
	"github.com/nikandfor/tlog/wire"
)

type (
	filereader struct {
		n string
		f *os.File
	}

	perrWriter struct {
		io.WriteCloser
	}

	pConnClose struct {
		net.PacketConn
		def []func() error
	}

	listenerClose struct {
		net.Listener
		def []func() error
	}
)

func main() {
	catCmd := &cli.Command{
		Name:   "convert,cat,c",
		Action: cat,
		Args:   cli.Args{},
		Flags: []*cli.Flag{
			cli.NewFlag("output,out,o", "-+dm", "output file (empty is stderr, - is stdout)"),
			cli.NewFlag("clickhouse", "", "send logs to clickhouse"),
			cli.NewFlag("follow,f", false, "wait for changes until terminated"),
			cli.NewFlag("head", 0, "skip all except first n events"),
			cli.NewFlag("tail", 0, "skip all except last n events"),
			cli.NewFlag("filter", "", "span filter"),
			cli.NewFlag("filter-depth", 0, "span filter max depth"),
		},
	}

	tlzCmd := &cli.Command{
		Name:        "seen,tlz",
		Description: "logs compressor/decompressor",
		Flags: []*cli.Flag{
			cli.NewFlag("output,o", "-", "output file (or stdout)"),
		},
		Commands: []*cli.Command{{
			Name:   "compress,c",
			Action: tlz,
			Args:   cli.Args{},
			Flags: []*cli.Flag{
				cli.NewFlag("block,b", 1*rotated.MiB, "compression block size"),
			},
		}, {
			Name:   "decompress,d",
			Action: tlz,
			Args:   cli.Args{},
		}, {
			Name:   "dump",
			Action: tlz,
			Args:   cli.Args{},
			Flags: []*cli.Flag{
				cli.NewFlag("base", 1, "global offset"),
			},
		}},
	}

	agentCmd := &cli.Command{
		Name:   "agent,run",
		Action: agentRun,
		Flags: []*cli.Flag{
			cli.NewFlag("db", "tlogdb", "path to logs db"),

			cli.NewFlag("listen,l", "", "listen address"),
			cli.NewFlag("listen-net,L", "unix", "listen network"),

			cli.NewFlag("listen-packet,p", "", "listen packet address"),
			cli.NewFlag("listen-packet-net,P", "unixgram", "listen packet network"),

			cli.NewFlag("http", ":8000", "http listen address"),
			cli.NewFlag("http-net", "tcp", "http listen network"),
		},
	}

	cli.App = cli.Command{
		Name:   "tlog",
		Before: before,
		Flags: []*cli.Flag{
			cli.NewFlag("log", "stderr", "log output file (or stderr)"),
			cli.NewFlag("verbosity,v", "", "logger verbosity topics"),
			cli.NewFlag("debug", "", "debug address"),
			cli.FlagfileFlag,
			cli.HelpFlag,
		},
		Commands: []*cli.Command{
			catCmd,
			tlzCmd,
			agentCmd,
			{
				Name:   "ticker",
				Action: ticker,
				Flags: []*cli.Flag{
					cli.NewFlag("output,o", "-", "output file (or stdout)"),
					cli.NewFlag("interval,int,i", time.Second, "interval to tick on"),
					cli.NewFlag("labels", "service=ticker,_pid", "labels"),
				},
			}, {
				Name:        "core",
				Description: "core dump memory dumper",
				Args:        cli.Args{},
				Action:      coredump,
			}, {
				Name:   "test",
				Action: test,
				Args:   cli.Args{},
			}},
	}

	err := cli.Run(os.Args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%+v\n", err)
		os.Exit(1)
	}
}

func before(c *cli.Command) error {
	w, err := tlflag.OpenWriter(c.String("log"))
	if err != nil {
		return errors.Wrap(err, "open log file")
	}

	tlog.DefaultLogger = tlog.New(w)

	tlog.SetFilter(c.String("verbosity"))

	if q := c.String("debug"); q != "" {
		go func() {
			tlog.Printw("start debug interface", "addr", q)

			err := http.ListenAndServe(q, nil)
			if err != nil {
				tlog.Printw("debug", "addr", q, "err", err, "", tlog.Fatal)
				os.Exit(1)
			}
		}()
	}

	//	gin.SetMode(gin.ReleaseMode)

	return nil
}

func agentRun(c *cli.Command) (err error) {
	ctx := context.Background()

	a, err := agent.New(c.String("db"))
	if err != nil {
		return errors.Wrap(err, "new agent")
	}

	g := graceful.New()

	if q := c.String("http"); q != "" {
		r := gin.New()

		r.Use(tlgin.Tracer)

		//	r.GET("/", s.HandleIndex)
		r.GET("/v0/*any", gin.WrapH(
			http.StripPrefix("/v0",
				a,
			),
		))

		l, err := net.Listen(c.String("http-net"), q)
		if err != nil {
			return errors.Wrap(err, "listen http: %v", q)
		}

		tlog.Printw("listen http", "addr", l.Addr())

		g.Add(ctx, "listen http", func(ctx context.Context) error {
			err := r.RunListener(l)
			select {
			case <-ctx.Done():
				return nil
			default:
				return err
			}
		}, graceful.WithStop(func(ctx context.Context) error {
			err := l.Close()
			return errors.Wrap(err, "close")
		}), graceful.WithCancelContext())
	}

	if q := c.String("listen"); q != "" {
		l, err := listen(c.String("listen-net"), q)
		if err != nil {
			return errors.Wrap(err, "listen stream")
		}

		tlog.Printw("listen stream", "addr", l.Addr())

		g.Add(ctx, "listen stream", func(ctx context.Context) error {
			err := a.Listen(ctx, l)
			select {
			case <-ctx.Done():
				return nil
			default:
				return err
			}
		}, graceful.WithStop(func(ctx context.Context) error {
			err := l.Close()
			return errors.Wrap(err, "close stream listener")
		}), graceful.WithCancelContext())
	}

	if q := c.String("listen-packet"); q != "" {
		l, err := listenPacket(c.String("listen-packet-net"), q)
		if err != nil {
			return errors.Wrap(err, "listen packet")
		}

		tlog.Printw("listen packet", "addr", l.LocalAddr())

		g.Add(ctx, "listen packet", func(ctx context.Context) error {
			err := a.ListenPacket(ctx, l)
			select {
			case <-ctx.Done():
				return nil
			default:
				return err
			}
		}, graceful.WithStop(func(ctx context.Context) error {
			err := l.Close()
			return errors.Wrap(err, "close packet listener")
		}), graceful.WithCancelContext())
	}

	return g.Run(ctx)
}

func setupHTTPDB(c *cli.Command, db *bbolt.DB) (err error) {
	tldb := tlbolt.NewWriter(db)

	h := func(c *gin.Context) {
		var n int

		if q := c.Query("n"); q != "" {
			v, err := strconv.ParseInt(q, 10, 32)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": errors.Wrap(err, "parse n").Error()})
				return
			}

			n = int(v)
		} else {
			n = -10
		}

		var token []byte
		if q := c.Query("token"); q != "" {
			token, err = hex.DecodeString(q)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": errors.Wrap(err, "parse token").Error()})
				return
			}
		}

		evs, next, err := tldb.Events("", int(n), token, nil)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.Header("X-Token-Next", hex.EncodeToString(next))
		c.Header("Content-Type", "application/json")

		w := convert.NewJSONWriter(c.Writer)

		w.TimeFormat = time.RFC3339Nano

		for _, ev := range evs {
			_, err = w.Write(ev)
			if err != nil {
				break
			}
		}
	}

	r := gin.New()

	v1 := r.Group("/v1/")

	v1.GET("/events", h)

	http.Handle("/v1/", r)

	return nil
}

func ticker(c *cli.Command) error {
	w, err := tlflag.OpenWriter(c.String("output"))
	if err != nil {
		return errors.Wrap(err, "open output")
	}

	w = perrWriter{
		WriteCloser: w,
	}

	l := tlog.New(w)

	t := time.NewTicker(c.Duration("interval"))
	defer t.Stop()

	ls := tlog.ParseLabels(c.String("labels"))

	l.SetLabels(ls)

	var first time.Time
	dur := c.Duration("interval")
	drift := 0.
	i := 0

	const alpha = 0.0001

	for t := range t.C {
		if i == 0 {
			first = t
		}

		diff := t.Sub(first) - time.Duration(i)*dur
		drift := drift*(1-alpha) + float64(diff)*alpha

		l.Printw("tick", "i", i, "time", t, "diff", diff, "drift", time.Duration(drift))

		i++
	}

	return nil
}

func cat(c *cli.Command) (err error) {
	var w io.WriteCloser

	var click *tlclickhouse.Writer
	if q := c.String("clickhouse"); q != "" {
		click, err = tlclickhouse.New(q)
		if err != nil {
			return errors.Wrap(err, "clickhouse")
		}

		w = tlio.NewTeeWriter(w, click)
	}

	if f := c.Flag("out"); click == nil || f.IsSet {
		wout, err := tlflag.OpenWriter(c.String("out"))
		if err != nil {
			return err
		}

		w = tlio.NewTeeWriter(w, wout)
	}

	if q := c.String("filter"); q != "" {
		p := processor.New(w, strings.Split(q, ",")...)
		p.MaxDepth = c.Int("filter-depth")

		w = tlio.WriteCloser{
			Writer: p,
			Closer: w,
		}
	}

	defer func() {
		e := w.Close()
		if err == nil {
			err = e
		}
	}()

	var fs *fsnotify.Watcher

	if c.Bool("follow") {
		fs, err = fsnotify.NewWatcher()
		if err != nil {
			return errors.Wrap(err, "create fs watcher")
		}

		defer func() {
			e := fs.Close()
			if err == nil {
				err = errors.Wrap(e, "close watcher")
			}
		}()
	}

	rs := make(map[string]io.WriterTo, c.Args.Len())
	defer func() {
		for name, r := range rs {
			if c, ok := r.(io.Closer); ok {
				e := c.Close()
				if err == nil {
					err = errors.Wrap(e, "close: %v", name)
				}
			}
		}
	}()

	for _, a := range c.Args {
		if fs != nil {
			fs.Add(a)
		}

		var r io.Reader
		r, err = tlflag.OpenReader(a)
		if err != nil {
			return errors.Wrap(err, "open: %v", a)
		}

		rs[a] = wire.NewStreamDecoder(r)

		var w0 io.Writer = w

		if f := c.Flag("tail"); f.IsSet {
			w0 = tlio.NewTailWriter(w0, *f.Value.(*int))
		}

		if f := c.Flag("head"); f.IsSet {
			fl, _ := w0.(tlio.Flusher)

			w0 = tlio.NewHeadWriter(w0, *f.Value.(*int))

			if _, ok := w0.(tlio.Flusher); !ok && fl != nil {
				w0 = tlio.WriteFlusher{
					Writer:  w0,
					Flusher: fl,
				}
			}
		}

		_, err = rs[a].WriteTo(w0)
		if errors.Is(err, io.EOF) {
			err = nil
		}

		if f, ok := w0.(interface{ Flush() error }); ok {
			e := f.Flush()
			if err == nil {
				err = errors.Wrap(e, "flush: %v", a)
			}
		}

		if err != nil {
			return errors.Wrap(err, "copy: %v", a)
		}
	}

	if !c.Bool("follow") {
		return nil
	}

	sigc := make(chan os.Signal, 3)
	signal.Notify(sigc, os.Interrupt)

	var ev fsnotify.Event
	for {
		select {
		case ev = <-fs.Events:
		case <-sigc:
			return nil
		case err = <-fs.Errors:
			return errors.Wrap(err, "watch")
		}

		//	tlog.Printw("fs event", "name", ev.Name, "op", ev.Op)

		switch {
		case ev.Op&fsnotify.Write != 0:
			r, ok := rs[ev.Name]
			if !ok {
				return errors.New("unexpected event: %v", ev.Name)
			}

			_, err = r.WriteTo(w)
			switch {
			case errors.Is(err, io.EOF):
			case errors.Is(err, io.ErrUnexpectedEOF):
				tlog.V("unexpected_eof").Printw("unexpected EOF", "file", ev.Name)
			case err != nil:
				return errors.Wrap(err, "copy: %v", ev.Name)
			}
		}
	}
}

func tlz(c *cli.Command) (err error) {
	var rs []io.Reader
	for _, a := range c.Args {
		if a == "-" {
			rs = append(rs, os.Stdin)
		} else {
			rs = append(rs, &filereader{n: a})
		}
	}

	if len(rs) == 0 {
		rs = append(rs, os.Stdin)
	}

	var w io.Writer
	if q := c.String("output"); q == "" || q == "-" {
		w = os.Stdout
	} else {
		f, err := os.Create(q)
		if err != nil {
			return errors.Wrap(err, "open output")
		}
		defer func() {
			e := f.Close()
			if err == nil {
				err = e
			}
		}()

		w = f
	}

	switch c.MainName() {
	case "compress":
		e := compress.NewEncoder(w, c.Int("block"))

		for _, r := range rs {
			_, err = io.Copy(e, r)
			if err != nil {
				return errors.Wrap(err, "copy")
			}
		}
	case "decompress":
		d := compress.NewDecoder(io.MultiReader(rs...))

		_, err = io.Copy(w, d)
		if err != nil {
			return errors.Wrap(err, "copy")
		}
	case "dump":
		d := compress.NewDumper(w) // BUG: dumper does not work with writes not aligned to tags

		d.GlobalOffset = int64(c.Int("base"))

		_, err = io.Copy(d, io.MultiReader(rs...))
		if err != nil {
			return errors.Wrap(err, "copy")
		}
	default:
		return errors.New("unexpected command: %v", c.MainName())
	}

	return nil
}

func coredump(c *cli.Command) (err error) {
	if c.Args.Len() != 1 {
		return errors.New("one arg expected")
	}

	f, err := elf.Open(c.Args.First())
	if err != nil {
		return errors.Wrap(err, "open")
	}

	sym, err := f.Symbols()
	if err != nil {
		return errors.Wrap(err, "get symbols")
	}

	for _, sym := range sym {
		tlog.Printw("symbol", "sym", sym)
	}

	return nil
}

func (f *filereader) Read(p []byte) (n int, err error) {
	if f.f == nil {
		f.f, err = os.Open(f.n)
		if err != nil {
			return 0, errors.Wrap(err, "open %v", f.n)
		}
	}

	n, err = f.f.Read(p)

	if err != nil {
		_ = f.f.Close()
	}

	return
}

func test(c *cli.Command) (err error) {
	//	low.Printw = tlog.Printw
	//	low.LoadGoTypes(c.Args.First())

	//	inf, ok := debug.ReadBuildInfo()
	//	tlog.Printw("build info", "info", inf, "ok", ok)

	return nil
}

func flock(f *os.File) (err error) {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
}

func (w perrWriter) Write(p []byte) (n int, err error) {
	n, err = w.WriteCloser.Write(p)

	if err != nil {
		tlog.Printw("write", "err", err)
	}

	return
}

func (w perrWriter) Close() (err error) {
	err = w.WriteCloser.Close()

	if err != nil {
		tlog.Printw("close", "err", err)
	}

	return
}

func listenPacket(netw, addr string) (p net.PacketConn, err error) {
	unix := strings.HasPrefix(netw, "unix")

	var def []func() error

	if unix {
		cl, err := flockLock(addr)
		if err != nil {
			return nil, errors.Wrap(err, "lock")
		}

		def = append(def, cl)
	}

	p, err = net.ListenPacket(netw, addr)
	if err != nil {
		return nil, errors.Wrap(err, "listen: %v", addr)
	}

	if unix {
		def = append(def, func() (err error) {
			err = os.Remove(addr)
			return errors.Wrap(err, "remote socket file")
		})
	}

	if def != nil {
		return pConnClose{
			PacketConn: p,
			def:        def,
		}, nil
	}

	return p, nil
}

func listen(netw, addr string) (l net.Listener, err error) {
	unix := strings.HasPrefix(netw, "unix")

	var def []func() error

	if unix {
		cl, err := flockLock(addr)
		if err != nil {
			return nil, errors.Wrap(err, "lock")
		}

		def = append(def, cl)
	}

	l, err = net.Listen(netw, addr)
	if err != nil {
		return nil, errors.Wrap(err, "listen: %v", addr)
	}

	if unix {
		def = append(def, func() (err error) {
			err = os.Remove(addr)
			return errors.Wrap(err, "remote socket file")
		})
	}

	if def != nil {
		return listenerClose{
			Listener: l,
			def:      def,
		}, nil
	}

	return l, nil
}

func flockLock(addr string) (cl func() error, err error) {
	lock, err := os.Create(addr + ".lock")
	if err != nil {
		return nil, errors.Wrap(err, "open lock")
	}
	cl = func() (err error) {
		err = lock.Close()
		return errors.Wrap(err, "close lock")
	}

	err = flock(lock)
	if err != nil {
		return cl, errors.Wrap(err, "lock file")
	}

	inf, err := os.Stat(addr)
	switch {
	case os.IsNotExist(err):
		// ok: all clear
		err = nil
	case err == nil && inf.Mode().Type() == os.ModeSocket:
		tlog.Printw("remove old socket file", "addr", addr)

		err = os.Remove(addr)
		if err != nil {
			return cl, errors.Wrap(err, "remove socket file")
		}
	case err != nil:
		return cl, errors.Wrap(err, "socket file info")
	default:
		return cl, errors.New("not a socket file")
	}

	return
}

func (p pConnClose) Close() (err error) {
	err = p.PacketConn.Close()

	for i := len(p.def) - 1; i >= 0; i-- {
		e := p.def[i]()
		if err == nil {
			err = e
		}
	}

	return
}
