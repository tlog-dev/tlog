package main

import (
	"context"
	"database/sql"
	"debug/elf"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/ClickHouse/clickhouse-go"
	"github.com/fsnotify/fsnotify"
	"github.com/gin-gonic/gin"
	"github.com/nikandfor/cli"
	"github.com/nikandfor/errors"
	"go.etcd.io/bbolt"

	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/agent"
	"github.com/nikandfor/tlog/compress"
	"github.com/nikandfor/tlog/convert"
	"github.com/nikandfor/tlog/ext/tlbolt"
	"github.com/nikandfor/tlog/ext/tlflag"
	"github.com/nikandfor/tlog/rotated"
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

	Flusher interface {
		Flush()
	}

	WriteFlusher struct {
		io.Writer
		Flusher
	}
)

func main() {
	catCmd := &cli.Command{
		Name:   "convert,conv,cat,c",
		Action: conv,
		Args:   cli.Args{},
		Flags: []*cli.Flag{
			cli.NewFlag("output,out,o", "-+dm", "output file (empty is stderr, - is stdout)"),
			cli.NewFlag("follow,f", false, "wait for changes until terminated"),
			cli.NewFlag("clickhouse", "", "additional clickhouse writer"),
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
				cli.NewFlag("block,b", 1*rotated.MB, "compression block size"),
			},
		}, {
			Name:   "decompress,d",
			Action: tlz,
			Args:   cli.Args{},
		}},
	}

	agentCmd := &cli.Command{
		Name:   "agent",
		Action: agentFn,
		Flags: []*cli.Flag{
			cli.NewFlag("listen-packet,p", "", "listen packet address"),
			cli.NewFlag("listen-packet-net,P", "unixgram", "listen packet network"),
			cli.NewFlag("http", ":8000", "http listen address"),
			cli.NewFlag("http-net", "tcp", "http listen network"),

			cli.NewFlag("max-events", 100000, "max events to keep in memory"),

			cli.NewFlag("listen,l", "/var/log/tlog.tl", "listen address"),
			cli.NewFlag("listen-net,L", "unix", "listen network"),
			cli.NewFlag("db", "/var/log/tlog.db", "db address"),
			cli.NewFlag("db-max-size", "1G", "max db size"),
			cli.NewFlag("db-max-age", 30*24*time.Hour, "max logs age"),
			cli.NewFlag("tolog", false, "dump events to log"),
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
				Name:   "testreader",
				Action: agent0,
				Args:   cli.Args{},
			}, {
				Name:   "ticker",
				Action: ticker,
				Flags: []*cli.Flag{
					cli.NewFlag("output,o", "-", "output file (or stdout)"),
					cli.NewFlag("interval,int,i", time.Second, "interval to tick on"),
				},
			}, {
				Name:        "core",
				Description: "core dump memory dumper",
				Args:        cli.Args{},
				Action:      coredump,
			}, {
				Name:   "test",
				Action: test,
			}},
	}

	err := cli.Run(os.Args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%+v\n", err)
		os.Exit(1)
	}
}

var logwriter io.Writer

func before(c *cli.Command) error {
	w, err := tlflag.OpenWriter(c.String("log"))
	if err != nil {
		return errors.Wrap(err, "open log file")
	}

	logwriter = w

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

	return nil
}

func agentFn(c *cli.Command) (err error) {
	errc := make(chan error, 10)

	a := agent.New()

	a.MaxEvents = c.Int("max-events")

	if q := c.String("http"); q != "" {
		l, err := net.Listen(c.String("http-net"), q)
		if err != nil {
			return errors.Wrap(err, "listen http: %v", q)
		}

		tlog.Printw("listen http", "addr", l.Addr())

		go func() {
			err := http.Serve(l, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				ctx := context.Background()

				var follow, addlabels bool
				n := 10

				cw := tlog.NewConsoleWriter(w, tlog.LdetFlags)

				vs := req.URL.Query()

				if _, ok := vs["color"]; ok {
					cw.Colorize = true
				}

				if _, ok := vs["follow"]; ok {
					follow = true
				}

				if _, ok := vs["addlabels"]; ok {
					addlabels = true
				}

				if q := vs.Get("n"); q != "" {
					n, err = strconv.Atoi(q)
					if err != nil {
						fmt.Fprintf(w, "parse n: %v\n", err)
						return
					}
				}

				err = a.Subscribe(ctx, WriteFlusher{
					Writer:  cw,
					Flusher: w.(Flusher),
				}, n, follow, addlabels)
				if err != nil {
					tlog.Printw("http client", "err", err)
				}
			}))

			errc <- errors.Wrap(err, "serve http")
		}()
	}

	if q := c.String("listen-packet"); q != "" {
		p, err := listenPacket(c.String("listen-packet-net"), q)
		if err != nil {
			return errors.Wrap(err, "listen packet")
		}

		defer func() {
			e := p.Close()
			if err == nil {
				err = errors.Wrap(e, "close")
			}
		}()

		tlog.Printw("listening", "addr", p.LocalAddr())

		go func() {
			err := a.ListenPacket(p)

			errc <- errors.Wrap(err, "listen packet")
		}()
	}

	if q := c.String("listen"); q != "" {
		l, err := listen(c.String("listen-net"), q)
		if err != nil {
			return errors.Wrap(err, "listen")
		}

		defer func() {
			e := l.Close()
			if err == nil {
				err = errors.Wrap(e, "close")
			}
		}()

		tlog.Printw("listening", "addr", l.Addr())

		go func() {
			err := a.Listen(l)

			errc <- errors.Wrap(err, "listen packet")
		}()
	}

	err = <-errc

	return err
}

func agent1(c *cli.Command) (err error) {
	db, err := bbolt.Open(c.String("db"), 0644, nil)
	if err != nil {
		return errors.Wrap(err, "open db")
	}

	if q := c.String("http"); q != "" {
		err = setupHTTPDB(c, db)
		if err != nil {
			return errors.Wrap(err, "setup http")
		}

		l, err := net.Listen(c.String("http-net"), q)
		if err != nil {
			return errors.Wrap(err, "listen http: %v", q)
		}

		tlog.Printw("listen http", "addr", l.Addr())

		go func() {
			err := http.Serve(l, nil)
			if err != nil {
				tlog.Printw("serve http", "err", err)
				os.Exit(1)
			}
		}()
	}

	addrNet := c.String("listen-net")
	addr := c.String("listen")

	if addrNet == "unix" {
		lock, err := os.Create(addr + ".lock")
		if err != nil {
			return errors.Wrap(err, "open lock")
		}
		defer lock.Close()

		err = flock(lock)
		if err != nil {
			return errors.Wrap(err, "lock file")
		}

		if inf, err := os.Stat(addr); os.IsNotExist(err) {
			// ok: all clear
		} else if err == nil && inf.Mode().Type() == os.ModeSocket {
			tlog.Printw("remove old socket file", "addr", addr)

			err = os.Remove(addr)
			if err != nil {
				return errors.Wrap(err, "remove socket file")
			}
		} else if err != nil {
			return errors.Wrap(err, "socket file info")
		} else {
			return errors.Wrap(err, "bad socket file type: %v", inf.Mode().Type())
		}
	}

	l, err := net.Listen(addrNet, addr)
	if err != nil {
		return errors.Wrap(err, "listen: %v", addr)
	}

	if addrNet == "unix" {
		defer os.Remove(addr)
	}

	tlog.Printw("listen", "addr", l.Addr())

	for {
		conn, err := l.Accept()
		if err != nil {
			return errors.Wrap(err, "accept")
		}

		go func() (err error) {
			tr := tlog.Start("accept", "remote_addr", conn.RemoteAddr(), "local_addr", conn.LocalAddr())
			defer func() {
				e := conn.Close()
				if e != nil {
					tr.Printw("close conn", "err", e)
				}

				tr.Finish("err", err)
			}()

			var r io.Reader = conn

			if ext := filepath.Ext(conn.LocalAddr().String()); ext == ".tlz" || ext == ".ez" || ext == ".seen" {
				r = compress.NewDecoder(r)
			}

			var w io.Writer

			w = tlbolt.NewWriter(db)

			if c.Bool("tolog") {
				w = tlog.TeeWriter{logwriter, w}
			}

			err = convert.Copy(w, r)
			if errors.Is(err, io.EOF) {
				err = nil
			}
			if err != nil {
				err = errors.Wrap(err, "convert")
			}

			return
		}()
	}

	return nil
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

func agent0(c *cli.Command) error {
	if c.Args.Len() == 0 {
		return errors.New("arguments expected")
	}

	f := c.Args.First()

	r, err := tlflag.OpenReader(f)
	if err != nil {
		return errors.Wrap(err, "open: %v", f)
	}

	d := wire.NewStreamDecoder(r)
	cnt := 0

	for {
		i := d.Keep(true)

		d.Skip()
		if errors.Is(d.Err(), io.EOF) {
			tlog.Printw("EOF. wait...")
			time.Sleep(500 * time.Millisecond)

			d.ResetErr()

			continue
		}
		if err = d.Err(); err != nil {
			return errors.Wrap(err, "reading event")
		}

		end := d.Pos()

		cnt++

		tlog.Printw("read event", "events", cnt, "st", i, "end", end)

		if false && cnt%3 == 0 {
			tlog.Printw("truncate file", "events", cnt, "st", i)
		}
	}
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

	l.SetLabels(tlog.Labels{"service=ticker"})

	i := 0

	for t := range t.C {
		l.Printw("tick", "i", i, "time", t)
		i++
	}

	return nil
}

func conv(c *cli.Command) (err error) {
	var w io.WriteCloser

	w, err = tlflag.OpenWriter(c.String("out"))
	if err != nil {
		return err
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

	rs := make(map[string]io.ReadCloser, c.Args.Len())
	defer func() {
		for name, r := range rs {
			if r != nil {
				e := r.Close()
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

		rs[a], err = tlflag.OpenReader(a)
		if err != nil {
			return errors.Wrap(err, "open: %v", a)
		}

		err = convert.Copy(w, rs[a])
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

		if ev.Op&fsnotify.Write != 0 {
			rc, ok := rs[ev.Name]
			if !ok {
				return errors.New("unexpected event: %v", ev.Name)
			}

			err = convert.Copy(w, rc)
			if err != nil {
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

	if c.MainName() == "compress" {
		e := compress.NewEncoder(w, c.Int("block"))

		for _, r := range rs {
			_, err = io.Copy(e, r)
			if err != nil {
				return errors.Wrap(err, "copy")
			}
		}
	} else {
		d := compress.NewDecoder(io.MultiReader(rs...))

		_, err = io.Copy(w, d)
		if err != nil {
			return errors.Wrap(err, "copy")
		}
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
	clickhouse.SetLogOutput(tlog.DefaultLogger.IOWriter(2))

	db, err := sql.Open("clickhouse", "tcp://localhost:9000?debug=1")
	if err != nil {
		return errors.Wrap(err, "connect to clickhouse")
	}

	tx, err := db.Begin()
	if err != nil {
		return errors.Wrap(err, "begin")
	}
	defer tx.Rollback()

	s1, err := tx.Prepare("INSERT INTO tlog (t_time)")
	if err != nil {
		return errors.Wrap(err, "prepare 1")
	}

	s2, err := tx.Prepare("INSERT INTO tlog (L)")
	if err != nil {
		return errors.Wrap(err, "prepare 2")
	}

	_, err = s1.Exec(time.Now())
	if err != nil {
		return errors.Wrap(err, "exec 1")
	}

	_, err = s2.Exec([]string{"a=b", "c"})
	if err != nil {
		return errors.Wrap(err, "exec 2")
	}

	err = tx.Commit()
	if err != nil {
		return errors.Wrap(err, "commit")
	}

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
