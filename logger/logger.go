package logger

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var Log *zap.Logger

func init() {
	// 🧱 File rotation setup
	fileWriter := zapcore.AddSync(&lumberjack.Logger{
		Filename:   "./logs/app.log", // path to your log file
		MaxSize:    10,               // megabytes before rotation
		MaxBackups: 5,                // number of old log files to keep
		MaxAge:     30,               // days to retain old logs
		Compress:   true,             // compress rotated logs (.gz)
	})

	consoleEncoder := zapcore.NewConsoleEncoder(zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.CapitalColorLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	})

	fileEncoder := zapcore.NewJSONEncoder(zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.CapitalLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	})

	core := zapcore.NewTee(
		zapcore.NewCore(consoleEncoder, zapcore.AddSync(zapcore.Lock(os.Stdout)), zap.DebugLevel),
		zapcore.NewCore(fileEncoder, fileWriter, zap.InfoLevel),
	)

	Log = zap.New(core, zap.AddCaller(), zap.AddStacktrace(zap.ErrorLevel))
	zap.ReplaceGlobals(Log)
}
