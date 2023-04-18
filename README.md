[![Documentation](https://pkg.go.dev/badge/github.com/nikandfor/tlog)](https://pkg.go.dev/github.com/nikandfor/tlog?tab=doc)
[![Go workflow](https://github.com/nikandfor/tlog/actions/workflows/go.yml/badge.svg)](https://github.com/nikandfor/tlog/actions/workflows/go.yml)
[![CircleCI](https://circleci.com/gh/nikandfor/tlog.svg?style=svg)](https://circleci.com/gh/nikandfor/tlog)
[![codecov](https://codecov.io/gh/nikandfor/tlog/tags/latest/graph/badge.svg)](https://codecov.io/gh/nikandfor/tlog)
[![Go Report Card](https://goreportcard.com/badge/github.com/nikandfor/tlog)](https://goreportcard.com/report/github.com/nikandfor/tlog)
![GitHub tag (latest SemVer)](https://img.shields.io/github/v/tag/nikandfor/tlog?sort=semver)

# tlog

At least it is a logger, but it is much more than that.
It is an observability events system.
Event is a log or tracing message, tracing span start or finish, metric value, or anything you need.
Tons of work has been done to make it effective yet comfortable to use.
The events are encoded in a machine-readable format to be processed in any way, instant or later.
Events could be printed as logs, combined to build distributed traces, filtered and sent to an alerting service, processed and analyzed, and more.

tlog is a new way of instrumentation. Log once use smart.

Explore [examples](examples) and [extensions](ext).

# Status

The logging API is pretty solid. Now I'm working mostly on backend parts, web interface, integrations.

# Quick Start

## Logger

```go
tlog.Printf("just like log.Printf")

tlog.Printw("but structured is", "much", "better")

type Req struct {
	Start time.Time
	Path  string
}

tlog.Printw("any value type is", "supported", &Req{Start: time.Now(), Path: "/resource/path"})

l := tlog.New(ioWriter)
l.Printw("yet another logger, seriously?")
```

## Debug Topics Instead of Log Levels

No need to choose between tons of unrelated Debug logs and scant Info logs.
Each event can be filtered precisely and filter can be changed at runtime.

```go
tlog.SetVerbosity("rawdb,dump_request")

tlog.V("rawdb").Printw("make db query", "query", query) // V is ispired by glog.V

if tlog.If("dump_request") {
	// some heavy calculations could also be here
	tlog.Printw("full request data", "request", request)
}

if tlog.If("full_token") {
	tlog.Printw("db token", "token", token)
} else {
	tlog.Printw("db token", "token", token.ID)
}
```

Filtering is very flexible.
You can select topics, functions, types, files, packages, topics in locations.
You can select all in the file and then unselect some functions, etc.

## Traces

Traces are vital if you have simultaneous requests or distributed request propagation.
So they integrated into the logger to have the best experience.

```go
func ServeRequest(req *Request) {
	span := tlog.Start("request_root", "client", req.RemoteAddr, "path", req.URL.Path)
	defer span.Finish()

	ctx := tlog.ContextWithSpan(req.Context(), span)

	doc, err := loadFromDB(ctx, req.URL.Path)
	// if err ...

	_ = doc
}

func loadFromDB(ctx context.Context, doc string) (err error) {
	parent := tlog.SpanFromContext(ctx)
	span := parent.V("dbops").Spawn("load_from_db", "doc", doc)
	defer func() {
		span.Finish("err", err) // record result error
	}()

	span.Printw("prepare query")
	// ...

	if dirtyPages > tooMuch {
		// record event to the local span or to the parent if the local was not selected
		span.Or(parent).Printw("too much of dirty pages", "durty_pages", dirtyPages,
			tlog.KeyLogLevel, tlog.Warn)
	}
}
```

Trace events are the same to log events, except they have IDs.
You do not need to add the same data to trace attributes and write them to logs. It's the same!

## Data Format

Events are just key-value associative arrays. All keys are optional, any can be added.
Some keys have special meaning, like event timestamp or log level.
But it's only a convention; representational parts primarily use it: console pretty text formatter moves time to the first column, for example.

The default format is a machine readable CBOR-like binary format. And the logger backend is just io.Writer.
Text, JSON, Logfmt converters are provided. Any other can be implemented.

There is also a special compression format: as fast and efficient as snappy
yet safe in a sense that each event (or batch write) emits single Write to the file (io.Writer actually).

# Performance

Performance was in mind from the very beginning. The idea is to emit as many events as you want and not to pay for that by performance.
In a typical efficient application CPU profile, the logger takes only 1-3% of CPU usage with no events economy.
Almost all allocations were eliminated. That means less work is done, no garbage collector pressure, and lower memory usage.
