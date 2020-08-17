[![Documentation](https://godoc.org/github.com/nikandfor/tlog?status.svg)](https://pkg.go.dev/github.com/nikandfor/tlog?tab=doc)
[![Build Status](https://travis-ci.com/nikandfor/tlog.svg?branch=master)](https://travis-ci.com/nikandfor/tlog)
[![CircleCI](https://circleci.com/gh/nikandfor/tlog.svg?style=svg)](https://circleci.com/gh/nikandfor/tlog)
[![codecov](https://codecov.io/gh/nikandfor/tlog/branch/master/graph/badge.svg)](https://codecov.io/gh/nikandfor/tlog)
[![GolangCI](https://golangci.com/badges/github.com/nikandfor/tlog.svg)](https://golangci.com/r/github.com/nikandfor/tlog)
[![Go Report Card](https://goreportcard.com/badge/github.com/nikandfor/tlog)](https://goreportcard.com/report/github.com/nikandfor/tlog)
![GitHub tag (latest SemVer)](https://img.shields.io/github/v/tag/nikandfor/tlog?sort=semver)

# tlog
TraceLog - distributed tracing and logging.

Explore [examples](examples).

# Status
It evolves as I use it. I still can change anything, but for now I'm quiet satisfied with most of details.

It's tested a bit but bugs are possible. Please report if find.

# Logger

Logging as usual.
```golang
tlog.Printf("message: %v", "arguments")
```

## Conditional logging
There is some kind of verbosity levels.
```golang
tlog.V("debug").Printf("DEBUG: conditional message")

if l := tlog.V("trace"); l != nil {
    p := 1 + 2 // complex calculations here that will not be executed if log level is not high enough
    l.Printf("result: %v", p)
}

tlog.Printf("unconditional message") // prints anyway
```

Actually it's not verbosity levels but debug topics. Each conditional operation have some topics it belongs to. And you can configure Logger precisely, which topics at which locations are enabled at each moment (concurrent usage/update is supported).
```golang
func main() {
    // ...
	tlog.DefaultLogger = tlog.New(tlog.NewConsoleWriter(os.Stderr, tlog.LstdFlags))
	if *filtersFlag != "" {
		tlog.DefaultLogger.AppendWriter(
			tlog.NewNamedWriter("debug", *filtersFlag, // filter name, initial value
				tlog.NewJSONWriter(
					rotated.Create("/tmp/log.json"))))
	}

	// ...
	// later you can change filter by name.
	tlog.SetNamedFilter("debug", newFilterValue)
}

// path/to/module/and/file.go

func (t *Conn) Send(/*...*/) {
    // ...
    tlog.V("encrypt").Printf("tls encoding debug data")
    // ...
    tlog.V("telemetry,debug").Printf("telemetry ...")
    // ...
    if l := tlog.V("read,error"); l != nil {
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
Logger can be created separately. All the same operations available there.
```golang
l := tlog.New(...)
l.Printf("unconditional")
l.V("topic").Printf("conditional")
```

## Location and StackTrace

Location in source code is recorded for each message you log (if you not disabled it). But you also may also capture some location or stack trace.
```golang
l := tlog.Caller(0) // 0 means current line
l = tlog.Caller(2) // 2 frames higher
s := tlog.StackTrace(2, 4) // skip 2 frames and record next 4
```
Then you may get function name, file name and file line for each frame.
```golang
funcName, fileName, fileLine := l.NameFileLine()
funcName, fileName, fileLine = s[2].NameFileLine()
tlog.Printf("called from here: %v", l.String())
tlog.Printf("crashed\n%v", tlog.StackTrace(0, 10))
```

## Writer

Writer is a backend of logger. It encodes messages and writes to the file, console, network connection or else.

### ConsoleWriter

It supports the same flags as stdlib `log` plus some extra.
```golang
var w io.Writer = os.Stderr // could be any writer
tlog.DefaultLogger = tlog.New(tlog.NewConsoleWriter(w, tlog.LstdFlags | tlog.Milliseconds))
```

### JSONWriter

Encodes logs in a compact way to analyze them later. It only needs `io.Writer`.
```golang
file, err := // ...
// if err ...
var w io.Writer = file // could be os.Stderr or net.Conn...
tlog.DefailtLogger = tlog.New(tlog.NewJSONWriter(w))
```

### ProtobufWriter

Even more compact and fast encoding.
```golang
_ = tlog.NewProtoWriter(w)
```

### TeeWriter

You also may use several writers at the same time.
```golang
cw := tlog.NewConsoleWriter(os.Stderr, tlog.LdetFlags)
jw := tlog.NewJSONWriter(file)
w := tlog.NewTeeWriter(cw, jw) // order is important. In that order messages will be passed to writers.
l := tlog.New(w)
// actually TeeWriter is already used inside tlog.New, so you can do:
l = tlog.New(cw, jw) // the same result as before
```

### The best writer ever

You can implement your own `tlog.Writer`.
```golang
Writer interface {
    Labels(ls Labels)
    SpanStarted(s Span, parent ID, l Location)
    SpanFinished(s Span, el time.Duration)
    Message(m Message, s Span)
}
```

# Tracing

It's hard to overvalue tracing when it comes to many parallel requests and especially when it's distributed system.
So tracing is here.
```golang
func Google(ctx context.Context, user, query string) (*Response, error) {
    tr := tlog.Start()
    defer tr.Finish()

    tr.Printf("user %s made query: %s", user, q)

    for _, b := range backends {
        go func(){
            subctx := tlog.ContextWithID(ctx, tr.ID)
            res := b.Search(subctx, u, q)
            // handle res
        }()
    }

    var res Response
    // wait for and take results of backends

    tr.Printf("%d Pages found on backends", len(res.Pages))

    // ...

    tr.Printf("advertisments added")

    res.TraceID = tr.ID // return it in HTTP Header or somehow. Later you can use it to find all subSpans

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
Traces may be used as metrics either. Analyzing time of messages you can measure how much each function elapsed, how much time has passed since one message to another.

**Important thing you should remember: `context.Context Values` are not passed through the network (`http.Request.WithContext` for example). You must pass `Span.ID` manually. Should not be hard, it's just an `[16]byte` and have helper methods.**

Analysing and visualising tool is going to be later.

Trace also can be used as `EventLog` (similar to https://godoc.org/golang.org/x/net/trace#EventLog)

# Tracer + Logger

The best part is that you don't need to pass the same useful information to logs and to traces like when you use two separate systems, it's done for you!
```golang
tr := tlog.Start()
defer tr.Finish()

tr.Printf("each time you print something to trace it appears in logs either")

tlog.Printf("but logs don't appear in traces")
```

# Performance

## Allocs
Allocations are one of the worst enemies of performance. So I fighted each alloc and each byte and even hacked runtime (see `unsafe.go`). So you'll get much more than stdlib `log` gives you almost for the same price.
```
goos: linux
goarch: amd64
pkg: github.com/nikandfor/tlog

# LstdFlags
BenchmarkLogLoggerStd-8                   	 2869803	       406 ns/op	      24 B/op	       2 allocs/op
BenchmarkTlogConsoleLoggerStd-8           	 4258173	       286 ns/op	      24 B/op	       2 allocs/op

# LdetFlags
BenchmarkLogLoggerDetailed-8              	  833001	      1425 ns/op	     240 B/op	       4 allocs/op
BenchmarkTlogConsoleDetailed-8            	 1000000	      1081 ns/op	      24 B/op	       2 allocs/op

# trace with one message
BenchmarkTlogTracesConsoleDetailed-8      	  429392	      2729 ns/op	      24 B/op	       2 allocs/op
BenchmarkTlogTracesJSON-8                 	  523032	      2293 ns/op	      24 B/op	       2 allocs/op
BenchmarkTlogTracesProto-8                	  494980	      2107 ns/op	      24 B/op	       2 allocs/op
BenchmarkTlogTracesProtoPrintRaw-8        	  584179	      1835 ns/op	       0 B/op	       0 allocs/op

# writers
BenchmarkWriterConsoleDetailedMessage-8   	 6566017	       185 ns/op	       0 B/op	       0 allocs/op
BenchmarkWriterJSONMessage-8              	16451940	        71.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkWriterProtoMessage-8             	18216289	        65.2 ns/op	       0 B/op	       0 allocs/op

# Caller
BenchmarkLocation-8               	 2987563	       396 ns/op	      32 B/op	       1 allocs/op
```
2 allocs in each line is `Printf` arguments: `int` to `interface{}` conversion and `[]interface{}` allocation.

2 more allocs in `LogLoggerDetailed` benchmark is because of `runtime.(*Frames).Next()` - that's why I hacked it.

## Writes

Writers designed to have one single write for each message you log, no more, no less. More writes per message is more operations and more system calls (if you write to `os.Stderr` or `*os.File`), so less performance and a risk to loose half of the message. Less writes (it means buffering multiple messages and write them together) is a chance to lose last messages in case of crash. And we don't want to lose message that describes reason why we crashed, do we?

# Roadmap
* Create swiss knife tool to analyse system performance through traces.
* Create interactive dashboard for traces with web interface.
