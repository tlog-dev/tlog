package tlogmain

import (
	"context"
	"debug/elf"
	"io"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/gin-gonic/gin"
	"github.com/nikandfor/cli"
	"github.com/nikandfor/errors"
	"github.com/nikandfor/graceful"

	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/agent"
	"github.com/nikandfor/tlog/compress"
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
		def []io.Closer
	}

	listenerClose struct {
		net.Listener
		def []io.Closer
	}
)

func App() *cli.Command {
	catCmd := &cli.Command{
		Name:        "convert,cat,c",
		Description: "read tlog encoded logs",
		Action:      cat,
		Args:        cli.Args{},
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
		Name:        "agent,run",
		Description: "run agent",
		Action:      agentRun,
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

	app := &cli.Command{
		Name:        "tlog",
		Description: "tlog cli",
		Before:      before,
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
				Name:        "ticker",
				Description: "simple test app that prints current time once in an interval",
				Action:      ticker,
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
				Hidden:      true,
			}, {
				Name:   "test",
				Action: test,
				Args:   cli.Args{},
				Hidden: true,
			}},
	}

	return app
}

func SubApp() *cli.Command {
	app := App()

	cp := *app
	cp.Before = nil
	cp.After = nil
	cp.Flags = nil

	return &cp
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

	if c.String("debug") == "" {
		gin.SetMode(gin.ReleaseMode)
	}

	return nil
}

func agentRun(c *cli.Command) (err error) {
	ctx := context.Background()

	root := agent.NewOSFS(c.String("db"))

	a, err := agent.New(root)
	if err != nil {
		return errors.Wrap(err, "new agent")
	}

	defer func() {
		e := a.Close()
		if err == nil {
			err = errors.Wrap(e, "close agent")
		}
	}()

	g := graceful.New()

	if q := c.String("http"); q != "" {
		r := gin.New()

		r.Use(tlgin.Tracer)

		r.GET("/", func(c *gin.Context) {
			c.Redirect(http.StatusSeeOther, "/v0")
		})

		//	r.GET("/", s.HandleIndex)
		r.GET("/v0/*any", gin.WrapH(
			http.StripPrefix("/v0", a),
		))

		l, err := net.Listen(c.String("http-net"), q)
		if err != nil {
			return errors.Wrap(err, "listen http: %v", q)
		}

		tlog.Printw("listen http", "addr", l.Addr())

		g.Add(func(ctx context.Context) error {
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
		}))
	}

	if q := c.String("listen"); q != "" {
		l, err := listen(c.String("listen-net"), q)
		if err != nil {
			return errors.Wrap(err, "listen stream")
		}

		tlog.Printw("listen stream", "addr", l.Addr())

		g.Add(func(ctx context.Context) error {
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
		}))
	}

	if q := c.String("listen-packet"); q != "" {
		l, err := listenPacket(c.String("listen-packet-net"), q)
		if err != nil {
			return errors.Wrap(err, "listen packet")
		}

		tlog.Printw("listen packet", "addr", l.LocalAddr())

		g.Add(func(ctx context.Context) error {
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
		}))
	}

	return g.Run(ctx)
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

	var def []io.Closer

	if unix {
		cl, err := flockLock(addr + ".lock")
		if err != nil {
			return nil, errors.Wrap(err, "lock")
		}

		def = append(def, cl)

		err = os.Remove(addr)
		if err != nil && !os.IsNotExist(err) {
			return nil, errors.Wrap(err, "remove old socket file")
		}
	}

	p, err = net.ListenPacket(netw, addr)
	if err != nil {
		return nil, errors.Wrap(err, "listen: %v", addr)
	}

	if unix {
		cf := func() (err error) {
			err = os.Remove(addr)
			return errors.Wrap(err, "remove unix socket file")
		}

		def = append(def, tlio.CloserFunc(cf))
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

	var def []io.Closer

	if unix {
		cl, err := flockLock(addr + ".lock")
		if err != nil {
			return nil, errors.Wrap(err, "lock")
		}

		def = append(def, cl)

		err = os.Remove(addr)
		if err != nil && !os.IsNotExist(err) {
			return nil, errors.Wrap(err, "remove old socket file")
		}
	}

	l, err = net.Listen(netw, addr)
	if err != nil {
		return nil, errors.Wrap(err, "listen: %v", addr)
	}

	// unix listener removed by close

	if def != nil {
		return listenerClose{
			Listener: l,
			def:      def,
		}, nil
	}

	return l, nil
}

func flockLock(addr string) (_ io.Closer, err error) {
	lock, err := os.OpenFile(addr, os.O_CREATE|syscall.O_EXLOCK|syscall.O_NONBLOCK, 0644)
	if err != nil {
		return nil, errors.Wrap(err, "open lock")
	}

	cl := func() (err error) {
		err = lock.Close()
		if err != nil {
			return errors.Wrap(err, "close lock")
		}

		err = os.Remove(addr)
		if err != nil {
			return errors.Wrap(err, "remove lock")
		}

		return nil
	}

	return tlio.CloserFunc(cl), nil
}

func flock(f *os.File) (err error) {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
}

func (p pConnClose) Close() (err error) {
	err = p.PacketConn.Close()

	for i := len(p.def) - 1; i >= 0; i-- {
		e := p.def[i].Close()
		if err == nil {
			err = e
		}
	}

	return
}

func (p listenerClose) Close() (err error) {
	err = p.Listener.Close()

	for i := len(p.def) - 1; i >= 0; i-- {
		e := p.def[i].Close()
		if err == nil {
			err = e
		}
	}

	return
}
