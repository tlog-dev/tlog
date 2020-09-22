// because of dependency errors
//+build ignore

package tlog

import (
	"testing"

	"github.com/opentracing/opentracing-go/log"
	"github.com/uber/jaeger-client-go"
)

type (
	jlog struct{}
)

func BenchmarkJaegerTracer(b *testing.B) {
	transp, err := jaeger.NewUDPTransport(":5000", 10000)
	if err != nil {
		b.Fatalf("new transport: %v", err)
	}

	rep := jaeger.NewRemoteReporter(transp)

	if false {
		rep = jaeger.NewCompositeReporter(rep, jaeger.NewLoggingReporter(jlog{}))
	}

	l, cl := jaeger.NewTracer("jaeger", jaeger.NewConstSampler(true), rep)
	defer cl.Close()

	for _, par := range []bool{false, true} {
		par := par

		n := SingleThread
		if par {
			n = Parallel
		}

		b.Run(n, func(b *testing.B) {
			gtr := l.StartSpan("global")

			for _, tc := range []struct {
				name string
				act  func(i int)
			}{
				{"StartFinish", func(i int) {
					tr := l.StartSpan("operation")
					tr.Finish()
				}},
				{"LogFields", func(i int) {
					gtr.LogFields(log.String("msg", "message"), log.Int("i", 1000+i)) // 1 alloc here: int to interface{} conversion
				}},
				{"StartLogFieldsFinish", func(i int) {
					tr := l.StartSpan("operation")
					gtr.LogFields(log.String("msg", "message"), log.Int("i", 1000+i)) // 1 alloc here: int to interface{} conversion
					tr.Finish()
				}},
			} {
				tc := tc

				b.Run(tc.name, func(b *testing.B) {
					b.ReportAllocs()
					//	fc.N, fc.B = 0, 0

					if par {
						b.RunParallel(func(b *testing.PB) {
							i := 0
							for b.Next() {
								i++
								tc.act(i)
							}
						})
					} else {
						for i := 0; i < b.N; i++ {
							tc.act(i)
						}
					}

					//	fc.ReportDisk(b)
				})
			}
		})
	}
}

func (jlog) Error(msg string)                    {}
func (jlog) Infof(f string, args ...interface{}) {}
