// Package logger provides a high-performance, structured logging abstraction
// based on uber-go/zap. It supports context-based logger propagation and
// child logger creation with structured fields.
package logger

import (
	"context"
	"os"
	"sync/atomic"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Logger wraps a zap sugared logger to provide a clean, structured API.
type Logger struct {
	s *zap.SugaredLogger
}

type ctxKey struct{}

// global is the process-wide logger set by Init. atomic.Pointer makes concurrent
// reads (FromContext, Get) and writes (Init) race-free.
var global atomic.Pointer[Logger]

// nop is a singleton no-op logger returned by FromContext before Init is called.
var nop = &Logger{s: zap.NewNop().Sugar()}

// Config defines the parameters for initializing the global logger.
type Config struct {
	// AppName is the identifier for the application in logs.
	AppName string
	// Level is the log level (debug, info, warn, error, fatal).
	Level string
}

// Init initializes the global logger with the provided configuration.
// It uses a JSON encoder and writes to standard output.
func Init(cfg Config) error {
	level := zapcore.InfoLevel
	if err := (&level).UnmarshalText([]byte(cfg.Level)); err != nil {
		level = zapcore.InfoLevel
	}

	enc := zapcore.NewJSONEncoder(zapcore.EncoderConfig{
		TimeKey:        "timestamp",
		LevelKey:       "level",
		MessageKey:     "message",
		CallerKey:      "caller",
		StacktraceKey:  "stacktrace",
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
	})

	core := zapcore.NewCore(enc, zapcore.AddSync(os.Stdout), zap.NewAtomicLevelAt(level))
	z := zap.New(core,
		zap.AddCaller(),
		zap.AddCallerSkip(1),
		zap.AddStacktrace(zapcore.ErrorLevel),
	)
	global.Store(&Logger{s: z.Sugar().With("app", cfg.AppName)})
	return nil
}

// Get returns the global logger. It returns nil if Init has not been called.
func Get() *Logger { return global.Load() }

// New wraps a *zap.Logger for use in tests or specialized scenarios.
func New(z *zap.Logger) *Logger {
	if z == nil {
		return nop
	}
	z = z.WithOptions(zap.AddCallerSkip(1))
	return &Logger{s: z.Sugar()}
}

// FromContext returns the logger stored in ctx, falling back to the global
// logger, or the package-level no-op logger if Init has not been called.
func FromContext(ctx context.Context) *Logger {
	if l, ok := ctx.Value(ctxKey{}).(*Logger); ok && l != nil {
		return l
	}
	if l := global.Load(); l != nil {
		return l
	}
	return nop
}

// WithContext stores the logger in the context and returns the new context.
func WithContext(ctx context.Context, l *Logger) context.Context {
	return context.WithValue(ctx, ctxKey{}, l)
}

// With returns a child logger with additional structured fields.
func (l *Logger) With(args ...any) *Logger {
	return &Logger{s: l.s.With(args...)}
}

// Info logs a message at the info level with optional structured fields.
func (l *Logger) Info(msg string, args ...any) { l.s.Infow(msg, args...) }

// Warn logs a message at the warn level with optional structured fields.
func (l *Logger) Warn(msg string, args ...any) { l.s.Warnw(msg, args...) }

// Error logs a message at the error level with optional structured fields.
func (l *Logger) Error(msg string, args ...any) { l.s.Errorw(msg, args...) }

// Debug logs a message at the debug level with optional structured fields.
func (l *Logger) Debug(msg string, args ...any) { l.s.Debugw(msg, args...) }

// Fatal logs a message at the fatal level and then calls os.Exit(1).
func (l *Logger) Fatal(msg string, args ...any) { l.s.Fatalw(msg, args...) }

// Sync flushes any buffered log entries.
func (l *Logger) Sync() { _ = l.s.Sync() }
