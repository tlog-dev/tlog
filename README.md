[![Documentation](https://godoc.org/github.com/nikandfor/tlog?status.svg)](http://godoc.org/github.com/nikandfor/tlog)
[![Build Status](https://travis-ci.com/nikandfor/tlog.svg?branch=master)](https://travis-ci.com/nikandfor/tlog)
[![CircleCI](https://circleci.com/gh/nikandfor/tlog.svg?style=svg)](https://circleci.com/gh/nikandfor/tlog)
[![codecov](https://codecov.io/gh/nikandfor/tlog/branch/master/graph/badge.svg)](https://codecov.io/gh/nikandfor/tlog)
[![GolangCI](https://golangci.com/badges/github.com/nikandfor/tlog.svg)](https://golangci.com/r/github.com/nikandfor/tlog)
[![Go Report Card](https://goreportcard.com/badge/github.com/nikandfor/tlog)](https://goreportcard.com/report/github.com/nikandfor/tlog)
![Project status](https://img.shields.io/badge/status-alpha-yellow.svg)

# tlog
TraceLog - distributed tracing and logging

# Status
It's still in active development phase. Not all features implemented yet but core features are here. Some API changes are possible. It evolves with my usage of it.

# Logger

Logging as usual.
```golang
tlog.Printf("message: %v", "arguments")
```

Verbosity is also supported. Error level is not propogated to the output. So you should prefix message yourself if you want.
```golang
tlog.V(tlog.LevDebug).Printf("DEBUG: conditional message")

if l := tlog.V(tlog.LevTrace); l != nil {
    p := 1 + 2 // complex calculations here that will not be executed if log level is not high enough
    l.Printf("result: %v", p)
}

tlog.Printf("unconditional message") // prints anyway
```

Logger can be created separately. All the same operations available there.
```golang
l := tlog.New(...)
l.Printf("unconditional")
l.V(LevError).Printf("conditional")
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

Encodes logs in a compact way to analyze them later. It only needs io.Writer
```golang
file, err := // ...
// if err ...
var w io.Writer = file // could be os.Stderr or net.Conn...
tlog.DefailtLogger = tlog.New(tlog.NewJSONWriter(w))
```

### TeeWriter

You also may use several writers at the same time.
```golang
cw := tlog.NewConsoleWriter(os.Stderr, tlog.LdetFlags)
jw := tlog.NewJSONWriter(file)
w := tlog.NewTeeWriter(cw, jw) // order is important. In that order messages will be passed to writers.
l := tlog.New(w)
```

### ProtobufWriter

Coming soon...

# Tracing

It's hard to overvalue tracing when it comes to many parallel requests and especially when it's distributed system.
So tracing is here.
```golang
func Google(ctx context.Context, user, q string) ([]string) {
    tr := tlog.Start()
    defer tr.Finish()
    
    tr.Printf("user %s made query: %s", user, q)
    
    c := make(chan string, len(backends))
    for _, b := range backends {
        go func(){
            subctx := tlog.ContextWithID(ctx, tr.SafeID())
            c <- b.Query(subctx, q)
        }()
    }
    
    var res []string
loop:
    for i := 0; i < len(backends); i++ {
        select {
        case r := <-c:
            res = append(res, r)
        case <-ctx.Done():
            break loop
        }
    }
    
    tlog.Printf("%d results received until timeout", len(res))
    
    traceID := tr.SafeID()
    // traceID could be retured in HTTP Header or as metainfo
    // Later you may use that traceID to find and isolate needed logs and spans.
    
    return res
}

func (b *VideosBackend) Search(ctx context.Context, q string) string {
    tr := tlog.SpawnFromContext(ctx)
    defer tr.Finish()

    // ...
    tr.Printf("anything")
    
    // ...

    return res
}
```
With traces you can measure timings such as how much each function elapsed, how much time has passed since one message to another.

**Important thing you should remember: context Values are not passed through the network (http.Request.WithContext for example). You must pass traceID manually. Luckily it's just an int64.**

Analysing and visualising tool is going to be later.

Trace also can be used as EventLog (similar to https://godoc.org/golang.org/x/net/trace#EventLog)

# Tracer + Logger

The best part is that you don't need to pass the same useful information to logs and to traces like when you use two separate libraries, it's done for you!
```golang
tr := tlog.Start()
defer tr.Finish()

tr.Printf("each time you print something to trace it appears in logs either")

tlog.Printf("but logs are not appeared in traces")
```

# Performance

I fighted each alloc and each byte and even hacked runtime (see `unsafe.go`). So you'll get much more than stdlib `log` gives you almost for the same price.
```
goos: linux
goarch: amd64
pkg: github.com/nikandfor/tlog
BenchmarkLogLoggerStd-8            	 3000000	       397 ns/op	      24 B/op	       2 allocs/op
BenchmarkTlogConsoleLoggerStd-8    	 2000000	       843 ns/op	      24 B/op	       2 allocs/op
BenchmarkLogLoggerDetailed-8       	 1000000	      1322 ns/op	     208 B/op	       4 allocs/op
BenchmarkTlogConsoleDetailed-8     	 1000000	      1240 ns/op	      24 B/op	       2 allocs/op
BenchmarkTlogTracesConsoleFull-8   	  500000	      3701 ns/op	     104 B/op	       3 allocs/op
BenchmarkTlogTracesJSONFull-8      	  500000	      3413 ns/op	     104 B/op	       3 allocs/op
```
2 allocs in each line is `Printf` arguments: `int` to `interface{}` conversion and `[]interface{}` allocation.

1 more alloc in `TlogTraces` benchmarks is `*Span` allocation.

2 more allocs in `LogLoggerDetailed` benchmark is because of `runtime.(*Frames).Next()` - that's why I hacked it.

# Roadmap
* Create swiss knife tool to analyse system performance through traces.
* Create interactive dashboard for traces with web interface.
