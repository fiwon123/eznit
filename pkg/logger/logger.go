package logger

import (
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/fiwon123/eznit/pkg/helper"
	"go.uber.org/zap"
	"go.uber.org/zap/exp/zapslog"
	"go.uber.org/zap/zapcore"
)

type Config struct {
	*slog.Logger
	zapLogger *zap.Logger
}

func NewConsole(logFolder string, enableDebug bool) (*Config, error) {

	// Console core – plain text, no timestamp/level (good for CLI)
	consoleEnc := zapcore.NewConsoleEncoder(zapcore.EncoderConfig{
		TimeKey:        "",
		LevelKey:       "level",
		MessageKey:     "msg",
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeLevel:    zapcore.CapitalLevelEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
	})

	level := zap.WarnLevel
	if enableDebug {
		level = zap.DebugLevel
	}

	consoleCore := zapcore.NewCore(consoleEnc, zapcore.AddSync(os.Stdout), level)

	cores := []zapcore.Core{consoleCore}

	if logFolder != "" {

		level := zap.InfoLevel
		if enableDebug {
			level = zap.DebugLevel
		}

		logFileCfg := zapcore.EncoderConfig{
			TimeKey:        "ts",
			LevelKey:       "level",
			MessageKey:     "msg",
			EncodeTime:     zapcore.ISO8601TimeEncoder,
			EncodeLevel:    zapcore.CapitalLevelEncoder,
			EncodeCaller:   zapcore.ShortCallerEncoder,
			EncodeDuration: zapcore.SecondsDurationEncoder,
			StacktraceKey:  "stacktrace",
		}

		err := helper.CreatePathIfNotExists(logFolder)
		if err != nil {
			return nil, err
		}

		logFile := filepath.Join(logFolder, time.Now().Format(time.DateOnly)+".log")

		var fileEnc zapcore.Encoder
		fileEnc = zapcore.NewConsoleEncoder(logFileCfg) // plain text with ts+level
		f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return nil, err
		}
		fileCore := zapcore.NewCore(fileEnc, zapcore.AddSync(f), level)
		cores = append(cores, fileCore)
	}

	// Combine cores
	tee := zapcore.NewTee(cores...)
	zapLogger := zap.New(tee, zap.AddCaller())

	handler := zapslog.NewHandler(zapLogger.Core(),
		zapslog.WithCaller(true),                 // include caller info
		zapslog.AddStacktraceAt(slog.LevelError), // stacktrace on error‑level logs
	)

	// Bridge to slog (so you can use slog.Info, slog.Error, etc.)
	slogLogger := slog.New(handler)

	slogLogger.Debug("Enabled Level Debug!")
	slogLogger.Info("logger initialized!", slog.Int("process_id", os.Getpid()))

	return &Config{
		Logger:    slogLogger,
		zapLogger: zapLogger,
	}, nil
}

func NewJson() {

}

func New(isJson bool, isDev bool) *Config {

	var config zap.Config
	var loggerLevel slog.Level

	loggerLevel = slog.LevelError
	if isDev {
		config = zap.NewDevelopmentConfig()
	} else {
		config = zap.NewProductionConfig()
	}

	var zapLogger *zap.Logger
	if isJson {
		config.Encoding = "json"
		zapLogger, _ = config.Build()
	} else {
		config.Encoding = "console"
		config.EncoderConfig.TimeKey = ""
		config.EncoderConfig.LevelKey = ""
		config.EncoderConfig.CallerKey = ""
		zapLogger, _ = config.Build()
	}

	logger := slog.New(zapslog.NewHandler(zapLogger.Core(),
		zapslog.WithCaller(true),
		zapslog.AddStacktraceAt(loggerLevel)))

	logger.Debug("Enabled Level Debug!")
	logger.Info("logger initialized!", slog.Int("process_id", os.Getpid()))

	return &Config{
		Logger:    logger,
		zapLogger: zapLogger,
	}
}

func (l *Config) Sync() {
	l.Info("Sync Logger...")
	l.zapLogger.Sync()
}
