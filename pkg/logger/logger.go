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

func New(isJson bool) *Config {
	var zapLogger *zap.Logger
	if isJson {
		zapLogger = zap.Must(zap.NewProduction())
	} else {
		zapLogger = zap.Must(zap.NewDevelopment())
	}
	defer zapLogger.Sync()

	logger := slog.New(zapslog.NewHandler(zapLogger.Core()))

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
