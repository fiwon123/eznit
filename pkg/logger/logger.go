package logger

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/exp/zapslog"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

type Config struct {
	*slog.Logger
	zapLogger *zap.Logger
	Sugar     *zap.SugaredLogger
}

func NewConsole(logFolder string, enableDebug bool) (*Config, error) {

	var cores []zapcore.Core

	// console
	encoder := getConsoleEncoder(false, false)
	consoleCore := initConsole(encoder, enableDebug)
	cores = append(cores, consoleCore)

	// log file
	encoder = getConsoleEncoder(true, true)
	fileCore, err := initLogFile(encoder, logFolder, enableDebug)
	if err != nil {
		return nil, err
	} else {
		cores = append(cores, fileCore)
	}

	// combine cores
	slogLogger, zapLogger := combineCore(cores)

	slogLogger.Debug("Enabled Level Debug!")
	slogLogger.Debug("logger initialized!", slog.Int("process_id", os.Getpid()))

	return &Config{
		Logger:    slogLogger,
		zapLogger: zapLogger,
		Sugar:     zapLogger.Sugar(),
	}, nil
}

func NewJson(logFolder string, enableDebug bool) (*Config, error) {
	var cores []zapcore.Core

	// console
	encoder := getJsonEncoder()
	consoleCore := initConsole(encoder, enableDebug)
	cores = append(cores, consoleCore)

	// log file
	encoder = getJsonEncoder()
	fileCore, err := initLogFile(encoder, logFolder, enableDebug)
	if err != nil {
		return nil, err
	} else {
		cores = append(cores, fileCore)
	}

	// combine cores
	slogLogger, zapLogger := combineCore(cores)

	slogLogger.Debug("Enabled Level Debug!")
	slogLogger.Debug("logger initialized!", slog.Int("process_id", os.Getpid()))

	return &Config{
		Logger:    slogLogger,
		zapLogger: zapLogger,
		Sugar:     zapLogger.Sugar(),
	}, nil
}

func (l *Config) Sync() {
	l.Debug("Sync Logger...")
	l.zapLogger.Sync()
}

func getConsoleEncoder(enableTime bool, enableStack bool) zapcore.Encoder {
	timeKey := ""
	if enableTime {
		timeKey = "ts"
	}

	stackKey := "stacktrace"
	if enableStack {
		stackKey = "stacktrace"

	}

	return zapcore.NewConsoleEncoder(zapcore.EncoderConfig{
		TimeKey:        timeKey,
		LevelKey:       "level",
		MessageKey:     "msg",
		StacktraceKey:  stackKey,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeLevel:    zapcore.CapitalLevelEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
	})
}

func getJsonEncoder() zapcore.Encoder {

	return zapcore.NewJSONEncoder(zapcore.EncoderConfig{
		TimeKey:       "ts",
		LevelKey:      "level",
		NameKey:       "logger",
		CallerKey:     "caller",
		MessageKey:    "msg",
		StacktraceKey: "stacktrace",
		LineEnding:    zapcore.DefaultLineEnding,
		EncodeLevel:   zapcore.CapitalLevelEncoder,
		EncodeTime: func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
			enc.AppendString(t.UTC().Format(time.RFC3339Nano))
		},
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	})
}

func initConsole(encoder zapcore.Encoder, enableDebug bool) zapcore.Core {
	level := zap.InfoLevel
	if enableDebug {
		level = zap.DebugLevel
	}

	return zapcore.NewCore(encoder, zapcore.AddSync(os.Stdout), level)
}

func initLogFile(encoder zapcore.Encoder, logFolder string, enableDebug bool) (zapcore.Core, error) {
	if logFolder == "" {
		return nil, fmt.Errorf("log folder is empty")
	}

	level := zap.InfoLevel
	if enableDebug {
		level = zap.DebugLevel
	}

	fileLogger := &lumberjack.Logger{
		Filename:   filepath.Join(logFolder, "current.log"),
		MaxSize:    200,
		MaxBackups: 7,
		MaxAge:     30,
		Compress:   true,
	}

	return zapcore.NewCore(encoder, zapcore.AddSync(fileLogger), level), nil
}

func combineCore(cores []zapcore.Core) (*slog.Logger, *zap.Logger) {
	tee := zapcore.NewTee(cores...)
	zapLogger := zap.New(tee, zap.AddCaller())

	handler := zapslog.NewHandler(zapLogger.Core(),
		zapslog.WithCaller(true),
		zapslog.AddStacktraceAt(slog.LevelError),
	)

	slogLogger := slog.New(handler)

	return slogLogger, zapLogger
}
