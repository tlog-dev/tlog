package tlog

import (
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func BenchmarkZapJSONInfo(b *testing.B) {
	b.ReportAllocs()

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

	for i := 0; i < b.N; i++ {
		l.Info("message", zap.Int("iter", i))
	}

	w.ReportDisk(b)
}

func BenchmarkZapJSONInfoParallel(b *testing.B) {
	b.ReportAllocs()

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

	b.RunParallel(func(b *testing.PB) {
		i := 0
		for b.Next() {
			i++
			l.Info("message", zap.Int("iter", i))
		}
	})

	w.ReportDisk(b)
}
