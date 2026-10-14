// Команда appmetrica — единственный production-бинарник: роли ingest, api, worker и служебные
// подкоманды migrate, admin, healthcheck.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/config"
	"appmetrica.online/backend/internal/httpx"
)

// version подставляется при сборке: -ldflags "-X main.version=<sha>".
var version = "dev"

const shutdownTimeout = 15 * time.Second

type command struct {
	name    string
	summary string
	run     func(ctx context.Context, args []string, stdout, stderr io.Writer) int
}

var commands = []command{
	{"ingest", "приём событий POST /v1/events (i.appmetrica.online)", runIngest},
	{"api", "Read API /v1 и admin API /admin/v1 (api.appmetrica.online)", runAPI},
	{"worker", "фоновые задачи: system_check, digest, close_day, deletion, session_cleanup", runWorker},
	{"migrate", "миграции Postgres и ClickHouse: migrate up|status", runMigrate},
	{"admin", "администрирование: admin passkey-link", runAdmin},
	{"healthcheck", "проверка /health для Docker HEALTHCHECK: healthcheck --url <url>", func(ctx context.Context, args []string, _, stderr io.Writer) int {
		return runHealthcheck(ctx, args, stderr)
	}},
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

// run возвращает код выхода: 0 — успех, 1 — ошибка выполнения, 2 — ошибка использования.
func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		usage(stderr)
		return 2
	}
	switch args[0] {
	case "-h", "-help", "--help", "help":
		usage(stdout)
		return 0
	case "-version", "--version", "version":
		say(stdout, "%s\n", version)
		return 0
	}
	for _, c := range commands {
		if c.name == args[0] {
			return c.run(ctx, args[1:], stdout, stderr)
		}
	}
	say(stderr, "appmetrica: неизвестная подкоманда %q\n\n", args[0])
	usage(stderr)
	return 2
}

func usage(w io.Writer) {
	say(w, "appmetrica %s\n\nИспользование: appmetrica <подкоманда> [флаги]\n\nПодкоманды:\n", version)
	for _, c := range commands {
		say(w, "  %-12s %s\n", c.name, c.summary)
	}
	say(w, "\nКонфигурация читается из переменных окружения, список — .env.example.\n")
}

// newLogger: в production — JSON, в development — текстовый формат.
func newLogger(w io.Writer, cfg config.Common, process string) *slog.Logger {
	opts := &slog.HandlerOptions{Level: cfg.LogLevel}
	var h slog.Handler = slog.NewJSONHandler(w, opts)
	if cfg.AppEnv == config.EnvDevelopment {
		h = slog.NewTextHandler(w, opts)
	}
	return slog.New(h).With(slog.String("process", process), slog.String("version", version))
}

// newRouter — общий каркас HTTP-процесса: /health, маршруты процесса (routes) и цепочка httpx.
func newRouter(logger *slog.Logger, c clock.Clock, routes ...func(mux *http.ServeMux)) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = io.WriteString(w, `{"status":"ok"}`)
	})
	for _, register := range routes {
		register(mux)
	}
	return httpx.Chain(mux,
		httpx.RequestID(c),
		httpx.AccessLog(logger, c),
		httpx.Recover(logger),
	)
}

// serveHTTP слушает addr до отмены ctx (SIGINT/SIGTERM), затем корректно завершает активные запросы.
func serveHTTP(ctx context.Context, logger *slog.Logger, addr string, h http.Handler) int {
	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", addr)
	if err != nil {
		logger.Error("listen failed", slog.String("addr", addr), slog.Any("error", err))
		return 1
	}
	srv := &http.Server{
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelWarn),
	}

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()
	logger.Info("listening", slog.String("addr", ln.Addr().String()))

	select {
	case err := <-errCh:
		logger.Error("server stopped", slog.Any("error", err))
		return 1
	case <-ctx.Done():
	}

	logger.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown failed", slog.Any("error", err))
		return 1
	}
	if err := <-errCh; err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("server stopped", slog.Any("error", err))
		return 1
	}
	return 0
}

// say пишет диагностику в stdout/stderr; ошибка записи в консоль не обрабатывается.
func say(w io.Writer, format string, a ...any) {
	_, _ = fmt.Fprintf(w, format, a...)
}

// parseFlags разбирает флаги подкоманды без аргументов-позиций; false — ошибка использования (код 2).
func parseFlags(fs *flag.FlagSet, args []string, stderr io.Writer) bool {
	fs.SetOutput(stderr)
	if err := fs.Parse(args); err != nil {
		return false
	}
	if fs.NArg() > 0 {
		say(stderr, "%s: лишние аргументы: %v\n", fs.Name(), fs.Args())
		return false
	}
	return true
}

// serveProcess — общий цикл процесса: основной HTTP-сервер, сервер метрик и фоновые задачи до отмены ctx.
// Остановка любого сервера останавливает остальные; код выхода ненулевой, если любой из них завершился
// с ошибкой — занятый порт метрик не должен выглядеть как штатная остановка.
func serveProcess(ctx context.Context, logger *slog.Logger, addr string, h http.Handler, metricsAddr string, metrics http.Handler, background ...func(ctx context.Context)) int {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var wg sync.WaitGroup
	for _, run := range background {
		wg.Go(func() { run(ctx) })
	}
	var metricsFailed atomic.Bool
	wg.Go(func() {
		if serveHTTP(ctx, logger, metricsAddr, metrics) != 0 {
			metricsFailed.Store(true)
			cancel()
		}
	})
	code := serveHTTP(ctx, logger, addr, h)
	cancel()
	wg.Wait() // фоновые задачи сбрасывают last_used_at и rejected_events до закрытия соединений
	if metricsFailed.Load() {
		return 1
	}
	return code
}
