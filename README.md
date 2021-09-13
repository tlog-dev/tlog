[![Documentation](https://pkg.go.dev/badge/github.com/nikandfor/tlog)](https://pkg.go.dev/github.com/nikandfor/tlog?tab=doc)
[![Build Status](https://travis-ci.com/nikandfor/tlog.svg?branch=master)](https://travis-ci.com/nikandfor/tlog)
[![CircleCI](https://circleci.com/gh/nikandfor/tlog.svg?style=svg)](https://circleci.com/gh/nikandfor/tlog)
[![codecov](https://codecov.io/gh/nikandfor/tlog/tags/latest/graph/badge.svg)](https://codecov.io/gh/nikandfor/tlog)
[![GolangCI](https://golangci.com/badges/github.com/nikandfor/tlog.svg)](https://golangci.com/r/github.com/nikandfor/tlog)
[![Go Report Card](https://goreportcard.com/badge/github.com/nikandfor/tlog)](https://goreportcard.com/report/github.com/nikandfor/tlog)
![GitHub tag (latest SemVer)](https://img.shields.io/github/v/tag/nikandfor/tlog?sort=semver)

# tlog

**Project is in process of rewriting.**

TraceLog is a new way of instrumentation. Log once use everywhere: logs, distributed traces, metrics, analytics and more.

Explore [examples](examples) and [extensions](ext).

# Contents
- [Status](#status)
- [Benifits](#benifits)
- [Logger](#logger)
  - [Structured](#structured-logging)
  - [Conditional](#conditional-logging)
  - [Logger object](#logger-object)
  - [In tests](#logging-in-tests)
  - [Callers](#callers)
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

# Status

It evolves as I use it. I still can change anything, but for now I'm quiet satisfied with most of details.

It's tested a bit but bugs are possible. Please report if find.

# Benifits

It's fast. I mean I've made number of improvements that gave another [50ns gain](https://pkg.go.dev/github.com/nikandfor/tlog/low#AppendPrintf) after all the usual methods had been defeated.
Almost all allocs are eliminated.
Only few are remain: `type` to `interface{}` conversion in `Attrs` and those that with less than linear growth from number of events.
That means no garbage collector pressure, no resources are spent on it.
No dependency from GC cycle, it does not have to [pay for alloc](https://blog.twitch.tv/en/2019/04/10/go-memory-ballast-how-i-learnt-to-stop-worrying-and-love-the-heap-26c2462549a2/#gc-assists) while gc works.
Benchmarks are [below](#performance).

It's powerful logger. Besides [stdlib like](https://golang.org/pkg/log/) logging there are
[structured logging](#structured-logging),
[conditional logging](#conditional-logging),
[testing logging](#logging-in-tests),
`Labels`, `Attributes`, `Callers` info attachment.

It combines all the instrumentation systems at one.
You do things once and use it reading logs, investigating traces, evaluating metrics, generating alerts.
Everything is [ready to work on distributed systems](#distributed) with no additional cost.

It's extendable from both sides. You can [implement `tlog.Writer`](#the-best-writer-ever) and process messages generated by `tlog.Logger`.
You can [create a wrapper](examples/extend/wrap.go) that will modify any aspect of logger. You can generate events and pass them to undelaying `tlog.Writer` directly.
Almost everything used internally is exported to be helpful for you.

It's going to mimic of and/or integrate with existing systems:
[prometheus](ext/tlprometheus/prometheus.go),
[OpenTelemetry](ext/tlotel/open_telemetry.go),
`net/trace`, Google Trace API, maybe jaeger backend, sentry...

It's going to have its own analysing tools taking advantage of a brand new events format.

It's open souce and free.

# Logger

Logging as usual.

```go
tlog.Printf("message: %v", "arguments")
```

## Structured logging

```go
tlog.Printw("message",
		"i", i,
		"path", pth)
```

## Conditional logging
There is some kind of verbosity levels.
```go
tlog.V("debug").Printf("DEBUG: conditional message")

if tlog.If("trace") {
    p := 1 + 2 // complex calculations here that will not be executed if log level is not high enough
    tlog.Printf("result: %v", p)
}

tlog.Printf("unconditional message") // prints anyway
```

Actually it's not verbosity levels but debug topics. Each conditional operation have some topics it belongs to. And you can configure Logger precisely, which topics at which locations are enabled at each moment (concurrent usage/update is supported).
```go
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

```go
l := tlog.New(...)

l.Printf("unconditional")
l.V("topic").Printf("conditional")

tr := l.Start("trace_name")
defer tr.Finish()

tr.Printf("trace info")
```

`nil` `*tlog.Logger` works perfectly fine as well as uninitialized `tlog.Span`. They both just do nothing but never `panic`.

```go
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

```go
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

## Callers

Location in source code is recorded for each message you log (if you not disabled it). But you also may capture some location or stack trace.

```go
l := loc.Caller(0) // 0 means current line
l = loc.Caller(2) // 2 frames higher
s := loc.Callers(2, 4) // skip 2 frames and record next 4

var globalVar loc.PC
l = loc.CallerOnce(0, &globalVar) // make heavy operation once; threadsafe
```

Then you may get function name, file name and file line for each frame.

```go
funcName, fileName, fileLine := l.NameFileLine()
funcName, fileName, fileLine = s[2].NameFileLine()
tlog.Printf("called from here: %#v", l)
tlog.Printf("crashed\n%v", tlog.Callers(0, 10))
```

# Writer

Default format is internal CBOR-like binary format. But converters are available.

Planned way is to log to a file (like normal loggers do) and to use separate agent
to process data, send it to external services or serve requests as part of distributed storage.

## ConsoleWriter

It supports the same flags as stdlib `log` plus some extra.
```go
var w io.Writer = os.Stderr // could be any writer
tlog.DefaultLogger = tlog.New(tlog.NewConsoleWriter(w, tlog.LstdFlags | tlog.Lmilliseconds))
```

## JSONWriter

Encodes logs in a compact way to analyze them later. It only needs `io.Writer`.
```go
file, err := // ...
// if err ...
var w io.Writer = file // could be os.Stderr or net.Conn...
tlog.DefailtLogger = tlog.New(convert.NewJSON(w))
```

## ProtoWriter

Ptotobuf encoding is compact and fast.
```go
_ = convert.NewProto(w)
```

## TeeWriter

You also may use several writers in the same time.
It works similar to `io.MultiWriter` but writes to all writers regardless of errors.

```go
cw := tlog.NewConsoleWriter(os.Stderr, tlog.LdetFlags)
jw := convert.NewJSON(file)
w := tlog.NewTeeWriter(cw, jw) // order is important. In that order messages will be passed to writers.
l := tlog.New(w)
```

## The best writer ever

You can implement your own [recoder](https://pkg.go.dev/github.com/nikandfor/tlog?tab=doc#Decoder).


There are more writers in `tlog` package, find them in [docs](https://pkg.go.dev/github.com/nikandfor/tlog?tab=doc).

# Tracer

It's hard to overvalue tracing when it comes to many parallel requests and especially when it's distributed system.
So tracing is here.

```go
func Google(ctx context.Context, user, query string) (*Response, error) {
    tr := tlog.Start("search_request") // records frame (function name, file and line) and start time
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

Trace can also be used as [net/trace.EventLog](https://pkg.go.dev/golang.org/x/net/trace#EventLog).

There is example middleware for [gin](ext/tlgin/gin.go) to extract `Span.ID` and spawn new `Span`

# Tracer + Logger

The best part is that you don't need to pass the same useful information to logs and to traces like when you use two separate systems, it's done for you!

```go
tr := tlog.Start("trace_name")
defer tr.Finish()

tr.Printf("each time you print something to trace it appears in logs either")

tlog.Printf("but logs don't appear in traces")
```

# Metrics

```go
tlog.SetLabels(tlog.Labels{"global=label"})

tlog.RegisterMetric("fully_qualified_metric_name_with_units", tlog.MetricSummary, "help message that describes metric")

// This is metric either. It records span duration as Metric.
tr := tlog.Start("scope_name")
defer tr.Finish()

tr.Printw("account", "account_id", accid)

tr.Observe("fully_qualified_metric_name_with_units", 123.456)
```

This result in the following prometheus-like output

```
# HELP fully_qualified_metric_name_with_units help message that describes metric
# TYPE fully_qualified_metric_name_with_units summary
fully_qualified_metric_name_with_units{global="label",quantile="0.1"} 123.456
fully_qualified_metric_name_with_units{global="label",quantile="0.5"} 123.456
fully_qualified_metric_name_with_units{global="label",quantile="0.9"} 123.456
fully_qualified_metric_name_with_units{global="label",quantile="0.95"} 123.456
fully_qualified_metric_name_with_units{global="label",quantile="0.99"} 123.456
fully_qualified_metric_name_with_units{global="label",quantile="1"} 123.456
fully_qualified_metric_name_with_units_sum{global="label"} 123.456
fully_qualified_metric_name_with_units_count{global="label"} 1
# HELP tlog_span_duration_ms span context duration in milliseconds
# TYPE tlog_span_duration_ms summary
tlog_span_duration_ms{global="label",func="main.main.func2",quantile="0.1"} 0.066248
tlog_span_duration_ms{global="label",func="main.main.func2",quantile="0.5"} 0.066248
tlog_span_duration_ms{global="label",func="main.main.func2",quantile="0.9"} 0.066248
tlog_span_duration_ms{global="label",func="main.main.func2",quantile="0.95"} 0.066248
tlog_span_duration_ms{global="label",func="main.main.func2",quantile="0.99"} 0.066248
tlog_span_duration_ms{global="label",func="main.main.func2",quantile="1"} 0.066248
tlog_span_duration_ms_sum{global="label",func="main.main.func2"} 0.066248
tlog_span_duration_ms_count{global="label",func="main.main.func2"} 1
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

```go
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

```go
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

Allocations are one of the worst enemies of performance. So I fighted each alloc and each byte and even hacked runtime (see `unsafe.go`).
So you'll get much more than stdlib `log` gives you almost for the same price.

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
