package logger

import (
	"log/slog"
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/exp/zapslog"
)

type Config struct {
	*slog.Logger
	zapLogger *zap.Logger
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
		zapLogger, _ = config.Build()
	}
	defer zapLogger.Sync()

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
