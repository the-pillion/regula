package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pillion/regula/internal/api"
	"github.com/pillion/regula/internal/config"
	"github.com/pillion/regula/internal/migrate"
	"github.com/pillion/regula/internal/seed"
	"github.com/pillion/regula/internal/store"
)

func main() {
	command := "serve"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}

	switch command {
	case "healthcheck":
		os.Exit(0)
	case "serve":
		serve()
	case "migrate":
		withRuntime(false, func(ctx context.Context, cfg *config.Config, logger *slog.Logger, pool *pgxpool.Pool, queries *store.Queries) error {
			return applyMigrations(ctx, logger, pool)
		})
	case "seed":
		seedCommand(os.Args[2:])
	case "reset-db":
		resetDBCommand(os.Args[2:])
	case "reset-and-seed":
		resetAndSeedCommand(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", command)
		os.Exit(1)
	}
}

func serve() {
	withRuntime(true, func(ctx context.Context, cfg *config.Config, logger *slog.Logger, pool *pgxpool.Pool, queries *store.Queries) error {
		handler, err := api.NewRouter(cfg, logger, queries, pool)
		if err != nil {
			return err
		}

		server := &http.Server{
			Addr:              ":" + strconv.Itoa(cfg.HTTPPort),
			Handler:           handler,
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       10 * time.Second,
			WriteTimeout:      15 * time.Second,
			IdleTimeout:       60 * time.Second,
		}

		go func() {
			logger.Info("regula listening", "addr", server.Addr)
			if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				logger.Error("http server failed", "error", err)
				os.Exit(1)
			}
		}()

		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
		<-sigs

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	})
}

func seedCommand(args []string) {
	manifestArg := resolveManifestArg(args)

	fs := flag.NewFlagSet("seed", flag.ExitOnError)
	manifest := fs.String("manifest", manifestArg, "path to seed manifest")
	_ = fs.Parse(stripPresetArg(args))

	withRuntime(false, func(ctx context.Context, cfg *config.Config, logger *slog.Logger, pool *pgxpool.Pool, queries *store.Queries) error {
		return seed.Apply(ctx, queries, *manifest)
	})
}

func resetDBCommand(args []string) {
	fs := flag.NewFlagSet("reset-db", flag.ExitOnError)
	yes := fs.Bool("yes", false, "confirm destructive database reset")
	_ = fs.Parse(args)

	if !*yes {
		fmt.Fprintln(os.Stderr, "reset-db requires --yes")
		os.Exit(1)
	}

	withRuntime(false, func(ctx context.Context, cfg *config.Config, logger *slog.Logger, pool *pgxpool.Pool, queries *store.Queries) error {
		if err := resetDB(ctx, pool); err != nil {
			return err
		}
		return applyMigrations(ctx, logger, pool)
	})
}

func resetAndSeedCommand(args []string) {
	manifestArg := resolveManifestArg(args)

	fs := flag.NewFlagSet("reset-and-seed", flag.ExitOnError)
	manifest := fs.String("manifest", manifestArg, "path to seed manifest")
	yes := fs.Bool("yes", false, "confirm destructive database reset")
	_ = fs.Parse(stripPresetArg(args))

	if !*yes {
		fmt.Fprintln(os.Stderr, "reset-and-seed requires --yes")
		os.Exit(1)
	}

	withRuntime(false, func(ctx context.Context, cfg *config.Config, logger *slog.Logger, pool *pgxpool.Pool, queries *store.Queries) error {
		if err := resetDB(ctx, pool); err != nil {
			return err
		}
		if err := applyMigrations(ctx, logger, pool); err != nil {
			return err
		}
		return seed.Apply(ctx, queries, *manifest)
	})
}

func manifestBaseDir() string {
	containerDir := filepath.Join(string(os.PathSeparator), "seed")
	if _, err := os.Stat(containerDir); err == nil {
		return containerDir
	}
	return "seed"
}

func defaultManifestPath() string {
	return filepath.Join(manifestBaseDir(), "foundation.json")
}

func resolveManifestArg(args []string) string {
	defaultPath := defaultManifestPath()
	if len(args) == 0 {
		return defaultPath
	}
	candidate := strings.TrimSpace(args[0])
	if candidate == "" || strings.HasPrefix(candidate, "-") {
		return defaultPath
	}
	if filepath.Ext(candidate) == "" {
		return filepath.Join(manifestBaseDir(), candidate+".json")
	}
	return candidate
}

func stripPresetArg(args []string) []string {
	if len(args) == 0 {
		return args
	}
	candidate := strings.TrimSpace(args[0])
	if candidate == "" || strings.HasPrefix(candidate, "-") {
		return args
	}
	return args[1:]
}

func withRuntime(autoMigrate bool, fn func(context.Context, *config.Config, *slog.Logger, *pgxpool.Pool, *store.Queries) error) {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}

	logger := newLogger(cfg.LogLevel)
	ctx := context.Background()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("database connection failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	pingCtx, pingCancel := context.WithTimeout(ctx, 10*time.Second)
	if err := pool.Ping(pingCtx); err != nil {
		pingCancel()
		logger.Error("database ping failed", "error", err)
		os.Exit(1)
	}
	pingCancel()

	if autoMigrate && cfg.AutoMigrate {
		if err := applyMigrations(ctx, logger, pool); err != nil {
			logger.Error("migration failed", "error", err)
			os.Exit(1)
		}
	}

	queries := store.New(pool)
	if err := fn(ctx, cfg, logger, pool, queries); err != nil {
		logger.Error("command failed", "error", err)
		os.Exit(1)
	}
}

func applyMigrations(ctx context.Context, logger *slog.Logger, pool *pgxpool.Pool) error {
	migrateCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := migrate.Apply(migrateCtx, pool); err != nil {
		return err
	}
	logger.Info("migrations applied")
	return nil
}

func resetDB(ctx context.Context, pool *pgxpool.Pool) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public; GRANT ALL ON SCHEMA public TO CURRENT_USER; GRANT ALL ON SCHEMA public TO public;`); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl}))
}
