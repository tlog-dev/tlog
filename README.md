[![Documentation](https://pkg.go.dev/badge/github.com/nikandfor/tlog)](https://pkg.go.dev/github.com/nikandfor/tlog?tab=doc)
[![Build Status](https://travis-ci.com/nikandfor/tlog.svg?branch=master)](https://travis-ci.com/nikandfor/tlog)
[![CircleCI](https://circleci.com/gh/nikandfor/tlog.svg?style=svg)](https://circleci.com/gh/nikandfor/tlog)
[![codecov](https://codecov.io/gh/nikandfor/tlog/branch/master/graph/badge.svg)](https://codecov.io/gh/nikandfor/tlog)
[![GolangCI](https://golangci.com/badges/github.com/nikandfor/tlog.svg)](https://golangci.com/r/github.com/nikandfor/tlog)
[![Go Report Card](https://goreportcard.com/badge/github.com/nikandfor/tlog)](https://goreportcard.com/report/github.com/nikandfor/tlog)
![GitHub tag (latest SemVer)](https://img.shields.io/github/v/tag/nikandfor/tlog?sort=semver)

# tlog
TraceLog - distributed tracing, logging and metrics.

Explore [examples](examples) and [extensions](ext).

Idea and most of the concepts were designated while working on distributed systems in [Yuri Korzhenevsky R&D Center](https://www.rnd.center).

# Contents
- [Status](#status)
- [Logger](#logger)
  - [Structured](#structured-logging)
  - [Conditional](#conditional-logging)
  - [Logger object](#logger-object)
  - [In tests](#logging-in-tests)
  - [Location and StackTrace](#location-and-stacktrace)
- [Writer](#writer)
  - [ConsoleWriter](#consolewriter)
  - [JSONWriter](#jsonwriter)
  - [ProtoWriter](#protowriter)
  - [TeeWriter](#teewriter)
  - [The best writer ever](#the-best-writer-ever)
- [Tracer](#tracer)
- [Tracer + Logger](#tracer--logger)
- [Metrics](#metrics)
- [Distributed](#distributed)
  - [Labels](#labels)
  - [Span.ID](#spanid)
- [Performance](#performance)
  - [Allocs](#allocs)
- [Roadmap](#roadmap)

# Status
It evolves as I use it. I still can change anything, but for now I'm quiet satisfied with most of details.

It's tested a bit but bugs are possible. Please report if find.

# Logger

Logging as usual.

```golang
tlog.Printf("message: %v", "arguments")
```

## Structured logging

```golang
tlog.Printw("message", tlog.AInt("i", i), tlog.AString("path", pth))

attrs := tlog.Attrs{
	{Name: "op", Value: "save"},
	{Name: "inter", Value: i}, // only basic types and tlog.ID are supported
}

// if ... { attrs = append(attrs, ...)

tlog.Printw("message", attrs...)
```

## Conditional logging
There is some kind of verbosity levels.
```golang
tlog.V("debug").Printf("DEBUG: conditional message")

if tlog.If("trace") {
    p := 1 + 2 // complex calculations here that will not be executed if log level is not high enough
    tlog.Printf("result: %v", p)
}

tlog.Printf("unconditional message") // prints anyway
```

Actually it's not verbosity levels but debug topics. Each conditional operation have some topics it belongs to. And you can configure Logger precisely, which topics at which locations are enabled at each moment (concurrent usage/update is supported).
```golang
func main() {
    // ...
	tlog.DefaultLogger = tlog.New(tlog.NewConsoleWriter(os.Stderr, tlog.LstdFlags))

	// change filter at any time.
	tlog.SetFilter(filter) // filter = "telemetry" or "Conn" or "Send=encrypt" or "file.go"
}

// path/to/module/and/file.go

func (t *Conn) Send(/*...*/) {
    // ...
    tlog.V("encrypt").Printf("tls encoding debug data")
    // ...
    tlog.V("telemetry,debug").Printf("telemetry ...")
    // ...
    if tlog.If("read,error") {
        // prepare and log message
    }
}
```

`filtersFlag` is a comma separated list of filters such as
```
# all messages with topics are enabled
telemetry
encryption
debug
trace

# all topics at specified location are enabled
path             # child packages are not enabled
path/*
path/to/file.go
file.go
package
(*Type)          # all Conn methods are enabled
Type             # short form
Type.Method      # one method
Method           # function or method of any object

# enable specific topics at specific location
package=encryption
Type=encryption+telemetry # multiple topics for location separated by '+'
```
List of filters is executed as chain of inclusion, exclusion and back inclusion of some locations.
```
path/*,!path/subpackage,path/subpackage/file.go,!funcInFile,!subpackage/file.go=debug+trace

What's happend:
* path/* - include whole subtree
* !path/subpackage - but exclude one of subpackages. Others: path/sub1/*, path/sub2/*, etc remain included.
* path/subpackage/file.go - but we interested in logs in file, so include it
* !funcInFile - except some function.
* !subpackage/file.go=debug+trace - and except topics `debug` and `trace` in file subpackage/file.go
```
In most cases it's enough to have only one filter, but if you need, you may have more with no performance loss.

By default all conditionals are disabled.

## Logger object

Logger can be created as an object by `tlog.New`.
All the same core functions are available at `*tlog.Logger`, `package` and `tlog.Span`.

```golang
l := tlog.New(...)

l.Printf("unconditional")
l.V("topic").Printf("conditional")

tr := l.Start()
defer tr.Finish()

tr.Printf("trace info")
```

`nil` `*tlog.Logger` works perfectly fine as well as uninitialized `tlog.Span`. They both just do nothing but never `panic`.

```golang
type Service struct {
	logger *tlog.Logger
}

func NewService() *Service {
	s := &Service{}

	if needLogs {
		s.logger = tlog.New(...) // set if needed, leave nil if not
	}

	return s
}

func (s *Service) Operation() {
	// ...
	s.logger.Printf("some details") // use anyway without fear
}
```

## Logging in tests

Log to `*testing.T` or `*testing.B` from your service code.

```golang
// using the Service defined above

func TestService(t *testing.T) {
	topics := "conn,rawbody" // get it from flags

	// if function crash messages from testing.T will not be printed
	// so set it to os.Stderr or buffer to print logs on your own
	// leave it nil to print to the test like by t.Logf
	var tostderr io.Writer

	tl := tlog.NewTestLogger(t, topics, tostderr)

	s := NewService()
	s.logger = tl

	r, err := s.PrepareOp()
	// if err != nil ...
	// assert r

	// instead of t.Logf()
	tl.Printf("dump: %v", r)

	// ...
}
```

## Location and StackTrace

Location in source code is recorded for each message you log (if you not disabled it). But you also may also capture some location or stack trace.
```golang
l := tlog.Caller(0) // 0 means current line
l = tlog.Caller(2) // 2 frames higher
s := tlog.Callers(2, 4) // skip 2 frames and record next 4
```
Then you may get function name, file name and file line for each frame.
```golang
funcName, fileName, fileLine := l.NameFileLine()
funcName, fileName, fileLine = s[2].NameFileLine()
tlog.Printf("called from here: %v", l.String())
tlog.Printf("crashed\n%v", tlog.Callers(0, 10))
```

# Writer

Writer is a backend of logger. It encodes messages and writes to the file, console, network connection or else.

Planned way is to log to the file (like normal loggers do) by compact encoding and to use separate agent to send it to central server or to serve requests as part of distributed storage.

## ConsoleWriter

It supports the same flags as stdlib `log` plus some extra.
```golang
var w io.Writer = os.Stderr // could be any writer
tlog.DefaultLogger = tlog.New(tlog.NewConsoleWriter(w, tlog.LstdFlags | tlog.Milliseconds))
```

## JSONWriter

Encodes logs in a compact way to analyze them later. It only needs `io.Writer`.
```golang
file, err := // ...
// if err ...
var w io.Writer = file // could be os.Stderr or net.Conn...
tlog.DefailtLogger = tlog.New(tlog.NewJSONWriter(w))
```

## ProtoWriter

Ptotobuf encoding is compact and fast.
```golang
_ = tlog.NewProtoWriter(w)
```

## TeeWriter

You also may use several writers at the same time.
```golang
cw := tlog.NewConsoleWriter(os.Stderr, tlog.LdetFlags)
jw := tlog.NewJSONWriter(file)
w := tlog.NewTeeWriter(cw, jw) // order is important. In that order messages will be passed to writers.
l := tlog.New(w)
// actually TeeWriter is already used inside tlog.New, so you can do:
l = tlog.New(cw, jw) // the same result as before
```

## The best writer ever

You can implement your own [tlog.Writer](https://pkg.go.dev/github.com/nikandfor/tlog?tab=doc#Writer).

```golang
Writer interface {
    Labels(ls Labels, sid ID) error
    SpanStarted(s SpanStart) error
    SpanFinished(f SpanFinish) error
    Message(m Message, sid ID) error
    Metric(m Metric, sid ID) error
    Meta(m Meta) error
}
```

There are more writers in `tlog` package, find them in [docs](https://pkg.go.dev/github.com/nikandfor/tlog?tab=doc).

# Tracer

It's hard to overvalue tracing when it comes to many parallel requests and especially when it's distributed system.
So tracing is here.

```golang
func Google(ctx context.Context, user, query string) (*Response, error) {
    tr := tlog.Start() // records start time and location (function name, file and line)
    defer tr.Finish() // records duration

    tr.SetLabels(Labels{"user=" + user}) // attach to Span and each of it's messages.
        // In contrast with (*Logger).SetLabels it can be called at any point.
	// Even after all messages and metrics.

    for _, b := range backends {
        go func(){
            subctx := tlog.ContextWithID(ctx, tr.ID)

            res := b.Search(subctx, u, q)

            // handle res
        }()
    }

    var res Response

    // wait for and take results of backends

    // each message contains time, so you can measure each block between messages
    tr.Printw("backends responded", tlog.AInt("pages", len(res.Pages)))

    // ...

    tr.Printf("advertisments added")

    // return it in HTTP Header or somehow.
    // Later you can use it to find all related Spans and Messages.
    res.TraceID = tr.ID

    return res, nil
}

func (b *VideosBackend) Search(ctx context.Context, q string) ([]*Page, error) {
    tr := tlog.SpawnFromContext(ctx)
    defer tr.Finish()

    // ...
    tr.Printf("anything")
    
    // ...

    return res, nil
}
```
Traces may be used as metrics either. Analyzing timestamps of messages you can measure how much time has passed since one message to another.

**Important thing you should remember: `context.Context Values` are not passed through the network (`http.Request.WithContext` for example). You must pass `Span.ID` manually.
Should not be hard, there are helpers.**

Analysing and visualising tool is going to be later.

Trace can also be used as [net/trace.EventLog](https://godoc.org/golang.org/x/net/trace#EventLog).

There is example middleware for [gin](ext/tlgin/gin.go) to extract `Span.ID` and spawn new `Span`

# Tracer + Logger

The best part is that you don't need to pass the same useful information to logs and to traces like when you use two separate systems, it's done for you!

```golang
tr := tlog.Start()
defer tr.Finish()

tr.Printf("each time you print something to trace it appears in logs either")

tlog.Printf("but logs don't appear in traces")
```

# Metrics

```golang
tlog.SetLabels(tlog.Labels{"global=label"})

tlog.RegisterMetric("fully_qualified_metric_name_with_units", tlog.MSummary, "help message that describes metric", tlog.Labels{"metric=const_label"})

// This is metric either. It records span duration as Metric.
tr := tlog.Start()
defer tr.Finish()

// write highly volatile values to messages, not to labels.
tr.Printf("account_id %x", accid)

// labels is not supposed to exceed 1000 unique key-values pairs.
tr.SetLabels(tlog.Labels{"span=label"})

tr.Observe("fully_qualified_metric_name_with_units", 123.456, tlog.Labels{"observation=label"})
```

This result in the following prometheus-like output

```
# HELP fully_qualified_metric_name_with_units help message that describes metric
# TYPE fully_qualified_metric_name_with_units summary
fully_qualified_metric_name_with_units{global="label",span="label",metric="const_label",observation="label",quantile="0.1"} 123.456
fully_qualified_metric_name_with_units{global="label",span="label",metric="const_label",observation="label",quantile="0.5"} 123.456
fully_qualified_metric_name_with_units{global="label",span="label",metric="const_label",observation="label",quantile="0.9"} 123.456
fully_qualified_metric_name_with_units{global="label",span="label",metric="const_label",observation="label",quantile="0.95"} 123.456
fully_qualified_metric_name_with_units{global="label",span="label",metric="const_label",observation="label",quantile="0.99"} 123.456
fully_qualified_metric_name_with_units{global="label",span="label",metric="const_label",observation="label",quantile="1"} 123.456
fully_qualified_metric_name_with_units_sum{global="label",span="label",metric="const_label",observation="label"} 123.456
fully_qualified_metric_name_with_units_count{global="label",span="label",metric="const_label",observation="label"} 1
# HELP tlog_span_duration_ms span context duration in milliseconds
# TYPE tlog_span_duration_ms summary
tlog_span_duration_ms{global="label",span="label",func="main.main.func2",quantile="0.1"} 0.094489
tlog_span_duration_ms{global="label",span="label",func="main.main.func2",quantile="0.5"} 0.094489
tlog_span_duration_ms{global="label",span="label",func="main.main.func2",quantile="0.9"} 0.094489
tlog_span_duration_ms{global="label",span="label",func="main.main.func2",quantile="0.95"} 0.094489
tlog_span_duration_ms{global="label",span="label",func="main.main.func2",quantile="0.99"} 0.094489
tlog_span_duration_ms{global="label",span="label",func="main.main.func2",quantile="1"} 0.094489
tlog_span_duration_ms_sum{global="label",span="label",func="main.main.func2"} 0.094489
tlog_span_duration_ms_count{global="label",span="label",func="main.main.func2"} 1
```

Check out prometheus naming convention https://prometheus.io/docs/practices/naming/.

# Distributed

Distributed tracing work almost the same as local logger.

## Labels

First thing you sould set up is `tlog.Labels`.
They are attached to each of the **following** `Message`, `Span` and `Metric`.
You can find out later which machine and process produced each log event by these labels.

Resetting `Labels` **replaces** all of them not just given. `Messages` created before `Labels` was set are not annotated with them.

There are some predefined label names that can be filled for you.

```golang
tlog.DefaultLogger = tlog.New(...)

// full list is in tlog.AutoLabels
base := tlog.FillLabelsWithDefaults("_hostname", "_user", "_pid", "_execmd5", "_randid")

ls := append(Labels{"service=myservice"}, base...)

ls = append(ls, tlog.ParseLabels(*userLabelsFlag)...)

tlog.SetLabels(ls)
```

## Span.ID

In a local code you may pass `Span.ID` in a `context.Context` as `tlog.ContextWithID` and derive from it as `tlog.SpawnFromContext`.
But additional actions are required in case of remote procedure call. You need to send `Span.ID` with arguments as a `string` or `[]byte`.
There are helper functions for that: `ID.FullString`, `tlog.IDFromString`, `ID[:]`, `tlog.IDFromBytes`.

Example for gin is here: [ext/tlgin/gin.go](ext/tlgin/gin.go)

```golang
func server(w http.ResponseWriter, req *http.Request) {
    xtr := req.Header.Get("X-Traceid")
    trid, err := tlog.IDFromString(xtr)
    if err != nil {
        trid = tlog.ID{}	    
    }
    
    tr := tlog.SpawnOrStart(trid)
    defer tr.Finish()

    if err != nil && xtr != "" {
        tr.Printf("bad trace id: %v %v", xtr, err)
    }

    // ...
}

func client(ctx context.Context) {
    req := &http.Request{}

    if id := tlog.IDFromContext(ctx); id != (tlog.ID{}) {
        req.Header.Set("X-Traceid", id.FullString()) // ID.String returns short prefix. It's not enough to Swawn from it.
    }
    
    // ...
}
```

# Performance

## Allocs

Allocations are one of the worst enemies of performance. So I fighted each alloc and each byte and even hacked runtime (see `unsafe.go`). So you'll get much more than stdlib `log` gives you almost for the same price.

```
goos: linux
goarch: amd64
pkg: github.com/nikandfor/tlog

# logging
BenchmarkStdLogLogger/Std/SingleThread-8       	 3347139	       351 ns/op	      24 B/op	       2 allocs/op
BenchmarkStdLogLogger/Std/Parallel-8           	 2244493	       515 ns/op	      24 B/op	       2 allocs/op
BenchmarkStdLogLogger/Det/SingleThread-8       	  935287	      1239 ns/op	     240 B/op	       4 allocs/op
BenchmarkStdLogLogger/Det/Parallel-8           	 1000000	      1288 ns/op	     240 B/op	       4 allocs/op

BenchmarkTlogLogger/Std/SingleThread/Printf-8         	 4355400	       280 ns/op	       0 B/op	       0 allocs/op
BenchmarkTlogLogger/Std/SingleThread/Printw-8         	 4105479	       294 ns/op	       8 B/op	       1 allocs/op
BenchmarkTlogLogger/Std/Parallel/Printf-8             	 7600929	       155 ns/op	       0 B/op	       0 allocs/op
BenchmarkTlogLogger/Std/Parallel/Printw-8             	 7674375	       156 ns/op	       8 B/op	       1 allocs/op
BenchmarkTlogLogger/Det/SingleThread/Printf-8         	 1000000	      1029 ns/op	       0 B/op	       0 allocs/op
BenchmarkTlogLogger/Det/SingleThread/Printw-8         	  953114	      1129 ns/op	       8 B/op	       1 allocs/op
BenchmarkTlogLogger/Det/Parallel/Printf-8             	 3991116	       315 ns/op	       0 B/op	       0 allocs/op
BenchmarkTlogLogger/Det/Parallel/Printw-8             	 3677959	       335 ns/op	       8 B/op	       1 allocs/op

BenchmarkZapLogger/SingleThread-8             	  576332	      1875 ns/op	     344 B/op	       4 allocs/op
BenchmarkZapLogger/Parallel-8                 	 2139580	       574 ns/op	     344 B/op	       4 allocs/op

BenchmarkGlogLogger/SingleThread-8          	  912760	      1325 ns/op	     224 B/op	       3 allocs/op
BenchmarkGlogLogger/Parallel-8              	 1943516	       629 ns/op	     224 B/op	       3 allocs/op

BenchmarkLogrusLogger/SingleThread-8         	  386980	      2786 ns/op	     896 B/op	      19 allocs/op
BenchmarkLogrusLogger/Parallel-8             	  263313	      5347 ns/op	     897 B/op	      19 allocs/op

# trace with one message 
BenchmarkTlogTraces/ConsoleStd/SingleThread/StartPrintfFinish-8   	  648499	      1837 ns/op	        36.8 disk_B/op	       0 B/op	       0 allocs/op
BenchmarkTlogTraces/ConsoleStd/Parallel/StartPrintfFinish-8       	 1615582	       718 ns/op	        36.5 disk_B/op	       0 B/op	       0 allocs/op
BenchmarkTlogTraces/JSON/SingleThread/StartPrintfFinish-8         	  444662	      2440 ns/op	       250 disk_B/op	       0 B/op	       0 allocs/op
BenchmarkTlogTraces/JSON/Parallel/StartPrintfFinish-8             	 1486056	       821 ns/op	       250 disk_B/op	       0 B/op	       0 allocs/op
BenchmarkTlogTraces/Proto/SingleThread/StartPrintfFinish-8        	  469704	      2306 ns/op	       114 disk_B/op	       0 B/op	       0 allocs/op
BenchmarkTlogTraces/Proto/Parallel/StartPrintfFinish-8            	 1578048	       763 ns/op	       113 disk_B/op	       0 B/op	       0 allocs/op

# writers
BenchmarkWriter/ConsoleDet/SingleThread/TracedMessage-8         	 5658282	       208 ns/op	        63.0 disk_B/op	       0 B/op	       0 allocs/op
BenchmarkWriter/ConsoleDet/SingleThread/TracedMetric-8          	  626032	      1893 ns/op	       104 disk_B/op	       0 B/op	       0 allocs/op
BenchmarkWriter/ConsoleDet/Parallel/TracedMessage-8             	 8096242	       148 ns/op	        63.0 disk_B/op	       0 B/op	       0 allocs/op
BenchmarkWriter/ConsoleDet/Parallel/TracedMetric-8              	 1942116	       623 ns/op	       104 disk_B/op	       0 B/op	       0 allocs/op
BenchmarkWriter/JSON/SingleThread/TracedMessage-8               	 9556735	       121 ns/op	        84.0 disk_B/op	       0 B/op	       0 allocs/op
BenchmarkWriter/JSON/SingleThread/TracedMetric-8                	 4357563	       276 ns/op	        65.0 disk_B/op	       0 B/op	       0 allocs/op
BenchmarkWriter/JSON/Parallel/TracedMessage-8                   	 6290318	       190 ns/op	        84.0 disk_B/op	       0 B/op	       0 allocs/op
BenchmarkWriter/JSON/Parallel/TracedMetric-8                    	 4307060	       280 ns/op	        65.0 disk_B/op	       0 B/op	       0 allocs/op
BenchmarkWriter/Proto/SingleThread/TracedMessage-8              	13024131	        85.1 ns/op	        49.0 disk_B/op	       0 B/op	       0 allocs/op
BenchmarkWriter/Proto/SingleThread/TracedMetric-8               	 9758936	       128 ns/op	        32.0 disk_B/op	       0 B/op	       0 allocs/op
BenchmarkWriter/Proto/Parallel/TracedMessage-8                  	 6444532	       187 ns/op	        49.0 disk_B/op	       0 B/op	       0 allocs/op
BenchmarkWriter/Proto/Parallel/TracedMetric-8                   	24649119	        44.0 ns/op	        32.0 disk_B/op	       0 B/op	       0 allocs/op

# Caller
BenchmarkLocationCaller-8         	 4326907	       265 ns/op	       0 B/op	       0 allocs/op
BenchmarkLocationNameFileLine-8   	 5736783	       207 ns/op	       0 B/op	       0 allocs/op
```
1 alloc in each line with `Printw` is `int` to `interface{}` conversion.

1 more alloc in most loggers is []interface{} allocation for variadic args. tlog is not the case because argumet doesn't leak and compiler optimiazation.
2 more allocs in `LogLogger/Det` benchmark is because of `runtime.(*Frames).Next()` - that's why I hacked it.

# Roadmap

* Create swiss knife tool to analyse system performance through traces.
* Create interactive dashboard for traces with web interface.
* Integrate with existing tools
