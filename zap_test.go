package tlog

import (
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func BenchmarkZapLogger(b *testing.B) {
	var w CountableDiscard

	enc := zapcore.NewJSONEncoder(zapcore.EncoderConfig{
		MessageKey:     "m",
		LevelKey:       "lv",
		TimeKey:        "t",
		CallerKey:      "l",
		NameKey:        "n",
		EncodeLevel:    zapcore.CapitalLevelEncoder,
		EncodeTime:     zapcore.EpochNanosTimeEncoder,
		EncodeDuration: zapcore.NanosDurationEncoder,
		EncodeCaller:   zapcore.FullCallerEncoder,
	})

	c := zapcore.NewCore(enc, zapcore.Lock(zapcore.AddSync(&w)), zapcore.DebugLevel)

	l := zap.New(c, zap.AddCaller())

	b.Run("SingleThread", func(b *testing.B) {
		b.ReportAllocs()
		w.N, w.B = 0, 0

		for i := 0; i < b.N; i++ {
			l.Info("message", zap.Int("i", 1000+i))
		}

		//	w.ReportDisk(b)
	})

	b.Run("Parallel", func(b *testing.B) {
		b.ReportAllocs()
		w.N, w.B = 0, 0

		b.RunParallel(func(b *testing.PB) {
			i := 0
			for b.Next() {
				i++
				l.Info("message", zap.Int("i", 1000+i))
			}
		})

		//	w.ReportDisk(b)
	})
}
