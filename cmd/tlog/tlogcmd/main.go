package tlogcmd

import (
	"context"
	"io"
	"io/fs"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"nikand.dev/go/cli"
	"nikand.dev/go/cli/flag"
	"nikand.dev/go/graceful"
	"nikand.dev/go/hacked/hnet"
	"tlog.app/go/eazy"
	"tlog.app/go/errors"

	"tlog.app/go/tlog"
	"tlog.app/go/tlog/agent"
	"tlog.app/go/tlog/ext/tlclick"
	"tlog.app/go/tlog/ext/tlflag"
	"tlog.app/go/tlog/tlio"
	"tlog.app/go/tlog/tlwire"
	"tlog.app/go/tlog/web"
)

type (
	filereader struct {
		n string
		f *os.File
	}

	perrWriter struct {
		io.WriteCloser
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
			cli.NewFlag("output,out,o", "-?dm", "output file (empty is stderr, - is stdout)"),
			cli.NewFlag("follow,f", false, "wait for changes until terminated"),
			cli.NewFlag("head", 0, "skip all except first n events"),
			cli.NewFlag("tail", 0, "skip all except last n events"),
			//	cli.NewFlag("filter", "", "span filter"),
			//	cli.NewFlag("filter-depth", 0, "span filter max depth"),
		},
	}

	tlzCmd := &cli.Command{
		Name:        "tlz,eazy",
		Description: "compressor/decompressor",
		Flags: []*cli.Flag{
			cli.NewFlag("output,o", "-", "output file (or stdout)"),
		},
		Commands: []*cli.Command{{
			Name:   "compress,c",
			Action: tlzRun,
			Args:   cli.Args{},
			Flags: []*cli.Flag{
				cli.NewFlag("block-size,block,bs,b", 1*eazy.MiB, "compression block size (window)"),
				cli.NewFlag("hash-table,ht", 1*1024, "hash table size"),
			},
		}, {
			Name:   "decompress,d",
			Action: tlzRun,
			Args:   cli.Args{},
		}, {
			Name:   "dump",
			Action: tlzRun,
			Args:   cli.Args{},
			Flags: []*cli.Flag{
				cli.NewFlag("base", -1, "global offset"),
			},
		}},
	}

	agentCmd := &cli.Command{
		Name:        "agent,run",
		Description: "run agent",
		Before:      beforeAgent,
		Action:      agentRun,
		Flags: []*cli.Flag{
			cli.NewFlag("db", "", "path to logs db"),
			cli.NewFlag("db-partition", 3*time.Hour, "db partition size"),
			cli.NewFlag("db-file-size", int64(eazy.GiB), "db file size"),
			cli.NewFlag("db-block-size", int64(16*eazy.MiB), "db file block size"),

			cli.NewFlag("clickdb", "", "clickhouse dsn"),

			cli.NewFlag("listen,l", []string(nil), "listen url"),

			cli.NewFlag("http", ":8000", "http listen address"),
			cli.NewFlag("http-net", "tcp", "http listen network"),
			cli.NewFlag("http-fs", "", "http templates fs"),

			cli.NewFlag("labels", "service=tlog-agent", "service labels"),
		},
	}

	app := &cli.Command{
		Name:        "tlog",
		Description: "tlog cli",
		Before:      before,
		Flags: []*cli.Flag{
			cli.NewFlag("log", "stderr?dm", "log output file (or stderr)"),
			cli.NewFlag("verbosity,v", "", "logger verbosity topics"),
			cli.NewFlag("debug", "", "debug address", flag.Hidden),
			cli.FlagfileFlag,
			cli.HelpFlag,
		},
		Commands: []*cli.Command{
			agentCmd,
			catCmd,
			tlzCmd,
			{
				Name:        "ticker",
				Description: "simple test app that prints current time once in an interval",
				Action:      ticker,
				Flags: []*cli.Flag{
					cli.NewFlag("output,o", "-", "output file (or stdout)"),
					cli.NewFlag("interval,int,i", time.Second, "interval to tick on"),
					cli.NewFlag("labels", "service=ticker,_pid", "labels"),
				},
			},
			{
				Name:   "test",
				Action: test,
				Args:   cli.Args{},
				Hidden: true,
			},
		},
	}

	return app
}

func SubApp() *cli.Command {
	app := App()

	app.Before = nil
	app.After = nil
	app.Flags = nil

	return app
}

func before(c *cli.Command) error {
	w, err := tlflag.OpenWriter(c.String("log"))
	if err != nil {
		return errors.Wrap(err, "open log file")
	}

	tlog.DefaultLogger = tlog.New(w)

	tlog.SetVerbosity(c.String("verbosity"))

	if q := c.String("debug"); q != "" {
		l, err := net.Listen("tcp", q)
		if err != nil {
			return errors.Wrap(err, "listen debug")
		}

		tlog.Printw("start debug interface", "addr", l.Addr())

		go func() {
			err := http.Serve(l, nil)
			if err != nil {
				tlog.Printw("debug", "addr", q, "err", err, "", tlog.Fatal)
				panic(err)
			}
		}()
	}

	if tlog.If("dump_file_reader") {
		tlflag.OpenFileReader = tlflag.OSOpenFile
		tlflag.OpenFileReader = tlflag.OpenFileDumpReader(tlflag.OpenFileReader)
		tlflag.OpenFileReader = tlflag.OpenFileReReader(tlflag.OpenFileReader)
	}

	return nil
}

func beforeAgent(c *cli.Command) error {
	if f := c.Flag("labels"); f != nil {
		if ls, ok := f.Value.(string); ok {
			tlog.SetLabels(tlog.ParseLabels(ls)...)
		}
	}

	return nil
}

func agentRun(c *cli.Command) (err error) {
	ctx := context.Background()
	ctx = tlog.ContextWithSpan(ctx, tlog.Root())

	if (c.String("db") != "") == (c.String("clickdb") != "") {
		return errors.New("exactly one of db and clickdb must be set")
	}

	var a interface {
		io.Writer
		web.Agent
	}

	if q := c.String("db"); q != "" {
		x, err := agent.New(q)
		if err != nil {
			return errors.Wrap(err, "new agent")
		}

		x.Partition = c.Duration("db-partition")
		x.FileSize = c.Int64("db-file-size")
		x.BlockSize = c.Int64("db-block-size")

		a = x
	} else if q := c.String("clickdb"); q != "" {
		opts := tlclick.DefaultPoolOptions(q)

		pool, err := tlclick.NewPool(ctx, opts)
		if err != nil {
			return errors.Wrap(err, "new click pool")
		}

		ch := tlclick.New(pool)

		err = ch.CreateTables(ctx)
		if err != nil {
			return errors.Wrap(err, "create click tables")
		}

		a = ch
	}

	group := graceful.New()

	if q := c.String("http"); q != "" {
		l, err := net.Listen(c.String("http-net"), q)
		if err != nil {
			return errors.Wrap(err, "listen http")
		}

		s, err := web.New(a)
		if err != nil {
			return errors.Wrap(err, "new web server")
		}

		if q := c.String("http-fs"); q != "" {
			s.FS = http.Dir(q)
		}

		group.Add(func(ctx context.Context) (err error) {
			tr := tlog.SpawnFromContext(ctx, "web_server", "addr", l.Addr())
			defer tr.Finish("err", &err)

			ctx = tlog.ContextWithSpan(ctx, tr)

			err = s.Serve(ctx, l, func(ctx context.Context, c net.Conn) (err error) {
				tr, ctx := tlog.SpawnFromContextAndWrap(ctx, "web_request", "remote_addr", c.RemoteAddr(), "local_addr", c.LocalAddr())
				defer tr.Finish("err", &err)

				return s.HandleConn(ctx, c)
			})
			if errors.Is(err, context.Canceled) {
				err = nil
			}

			return errors.Wrap(err, "serve http")
		})
	}

	for _, lurl := range c.Flag("listen").Value.([]string) {
		u, err := tlflag.ParseURL(lurl)
		if err != nil {
			return errors.Wrap(err, "parse %v", lurl)
		}

		tlog.Printw("listen", "scheme", u.Scheme, "host", u.Host, "path", u.Path, "query", u.RawQuery)

		host := u.Host
		if u.Scheme == "unix" || u.Scheme == "unixgram" {
			host = u.Path
		}

		l, p, err := listen(u.Scheme, host)
		if err != nil {
			return errors.Wrap(err, "listen %v", host)
		}

		switch {
		case u.Scheme == "unix", u.Scheme == "tcp":
			group.Add(func(ctx context.Context) error {
				var wg sync.WaitGroup

				defer wg.Wait()

				for {
					c, err := hnet.Accept(ctx, l)
					if errors.Is(err, context.Canceled) {
						return nil
					}
					if err != nil {
						return errors.Wrap(err, "accept")
					}

					wg.Add(1)

					tr := tlog.SpawnFromContext(ctx, "agent_writer", "local_addr", c.LocalAddr(), "remote_addr", c.RemoteAddr())

					go func() {
						defer wg.Done()

						var err error

						defer tr.Finish("err", &err)

						defer closeWrap(c, &err, "close conn")

						if f, ok := a.(tlio.Flusher); ok {
							defer doWrap(f.Flush, &err, "flush db")
						}

						rr := tlwire.NewReader(c)

						_, err = rr.WriteTo(a)
					}()
				}
			}, graceful.WithStop(func(ctx context.Context) error {
				return l.Close()
			}))
		case u.Scheme == "unixgram", u.Scheme == "udp":
			group.Add(func(ctx context.Context) error {
				buf := make([]byte, 0x1000)

				for {
					n, _, err := hnet.ReadFrom(ctx, p, buf)
					if err != nil {
						return errors.Wrap(err, "read")
					}

					_, _ = a.Write(buf[:n])
				}
			})
		default:
			return errors.New("unsupported listener: %v", u.Scheme)
		}
	}

	return group.Run(ctx, graceful.IgnoreErrors(context.Canceled))
}

func cat(c *cli.Command) (err error) {
	w, err := tlflag.OpenWriter(c.String("out"))
	if err != nil {
		return err
	}

	defer func() {
		e := w.Close()
		if err == nil {
			err = e
		}
	}()

	if tlog.If("describe,describe_writer") {
		tlflag.Describe(tlog.Root(), w)
	}

	var fs *fsnotify.Watcher //nolint:gocritic

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
			tlio.CloseWrap(r, name, &err)
		}
	}()

	var addFile func(a string) error
	addFile = func(a string) (err error) {
		a = filepath.Clean(a)

		inf, err := os.Stat(a)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return errors.Wrap(err, "stat %v", a)
		}

		if err == nil {
			if fs != nil {
				err = fs.Add(a)
				tlog.V("watch").Printw("watch file", "name", a, "err", err)
				if err != nil {
					return errors.Wrap(err, "watch")
				}
			}

			if inf.IsDir() {
				files, err := os.ReadDir(a)
				if err != nil {
					return errors.Wrap(err, "readdir %v", a)
				}

				for _, f := range files {
					if strings.HasPrefix(f.Name(), ".") {
						continue
					}
					if !f.Type().IsRegular() {
						continue
					}

					err = addFile(filepath.Join(a, f.Name()))
					if err != nil {
						return err
					}
				}

				return nil
			}
		}

		rc, err := tlflag.OpenReader(a)
		if err != nil {
			return errors.Wrap(err, "open reader")
		}

		if tlog.If("describe,describe_reader") {
			tlflag.Describe(tlog.Root(), rc)
		}

		rs[a] = tlwire.NewReader(rc)

		var w0 io.Writer = w

		if f := c.Flag("tail"); f.IsSet {
			w0 = tlio.NewTailWriter(w0, f.Value.(int))
		}

		if f := c.Flag("head"); f.IsSet {
			fl, _ := w0.(tlio.Flusher)

			w0 = tlio.NewHeadWriter(w0, f.Value.(int))

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

		if f, ok := w0.(tlio.Flusher); ok {
			e := f.Flush()
			if err == nil {
				err = errors.Wrap(e, "flush: %v", a)
			}
		}

		if err != nil {
			return errors.Wrap(err, "copy: %v", a)
		}

		return nil
	}

	for _, a := range c.Args {
		err = addFile(a)
		if err != nil {
			return err
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

		tlog.V("fsevent").Printw("fs event", "name", ev.Name, "op", ev.Op)

		switch {
		case ev.Op&fsnotify.Create != 0:
			err = addFile(ev.Name)
			if err != nil {
				return errors.Wrap(err, "add created")
			}
		case ev.Op&fsnotify.Remove != 0:
		//	err = fs.Remove(ev.Name)
		//	if err != nil {
		//		return errors.Wrap(err, "remove watch")
		//	}
		case ev.Op&fsnotify.Write != 0:
			r, ok := rs[ev.Name]
			if !ok {
				tlog.V("unexpected_event").Printw("unexpected event", "file", ev.Name, "map", rs)
				break
				//	return errors.New("unexpected event: %v (%v)", ev.Name, rs)
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

func tlzRun(c *cli.Command) (err error) {
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
		e := eazy.NewWriter(w, c.Int("block"), c.Int("hash-table"))

		for _, r := range rs {
			_, err = io.Copy(e, r)
			if err != nil {
				return errors.Wrap(err, "copy")
			}
		}
	case "decompress":
		d := eazy.NewReader(io.MultiReader(rs...))

		_, err = io.Copy(w, d)
		if err != nil {
			return errors.Wrap(err, "copy")
		}
	case "dump":
		d := eazy.NewDumper(w) // BUG: dumper does not work with writes not aligned to tags

		d.GlobalOffset = int64(c.Int("base"))

		data, err := io.ReadAll(io.MultiReader(rs...))
		if err != nil {
			return errors.Wrap(err, "read all")
		}

		_, err = d.Write(data)
		if err != nil {
			return errors.Wrap(err, "dumper")
		}

		err = d.Close()
		if err != nil {
			return errors.Wrap(err, "close dumper")
		}
	default:
		return errors.New("unexpected command: %v", c.MainName())
	}

	return nil
}

func ticker(c *cli.Command) error {
	w, err := tlflag.OpenWriter(c.String("output"))
	if err != nil {
		return errors.Wrap(err, "open output")
	}

	if tlog.If("describe,describe_writer") {
		tlflag.Describe(tlog.Root(), w)
	}

	w = perrWriter{
		WriteCloser: w,
	}

	l := tlog.New(w)

	t := time.NewTicker(c.Duration("interval"))
	defer t.Stop()

	ls := tlog.ParseLabels(c.String("labels"))

	l.SetLabels(ls...)

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

func test(c *cli.Command) error {
	return nil
}

func (f *filereader) Read(p []byte) (n int, err error) {
	if f.f == nil {
		f.f, err = os.Open(f.n)
		if err != nil {
			return 0, errors.Wrap(err, "")
			//	return 0, errors.Wrap(err, "open %v", f.n)
		}
	}

	n, err = f.f.Read(p)
	if err != nil {
		_ = f.f.Close()
	}

	return
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

func isDir(name string) bool {
	inf, err := os.Stat(name)
	if err != nil {
		return false
	}

	return inf.IsDir()
}

func isFifo(name string) bool {
	inf, err := os.Stat(name)
	if err != nil {
		return false
	}

	mode := inf.Mode()

	return mode&fs.ModeNamedPipe != 0
}

func listen(netw, addr string) (l net.Listener, p net.PacketConn, err error) {
	switch netw {
	case "unix", "unixgram":
		_ = os.Remove(addr)
	}

	switch netw {
	case "tcp", "unix":
		l, err = net.Listen(netw, addr)
		if err != nil {
			return nil, nil, errors.Wrap(err, "listen")
		}

		switch l := l.(type) {
		case *net.UnixListener:
			l.SetUnlinkOnClose(true)
		default:
			return nil, nil, errors.New("unsupported listener type: %T", l)
		}
	case "udp", "unixgram":
		p, err = net.ListenPacket(netw, addr)
		if err != nil {
			return nil, nil, errors.Wrap(err, "listen packet")
		}
	default:
		return nil, nil, errors.New("unsupported network type: %v", netw)
	}

	return l, p, nil
}

func (p listenerClose) SetDeadline(t time.Time) error {
	return p.Listener.(interface{ SetDeadline(time.Time) error }).SetDeadline(t)
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

func closeWrap(c io.Closer, errp *error, msg string) {
	doWrap(c.Close, errp, msg)
}

func flushWrap(x interface{}, errp *error, msg string) {
	if f, ok := x.(tlio.Flusher); ok {
		doWrap(f.Flush, errp, msg)
	}
}

func doWrap(f func() error, errp *error, msg string) {
	e := f()
	if *errp == nil {
		*errp = errors.Wrap(e, msg)
	}
}

func closeIfErr(c io.Closer, errp *error) {
	if *errp == nil {
		return
	}

	_ = c.Close()
}
