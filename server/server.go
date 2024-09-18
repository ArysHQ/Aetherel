package server

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aryshq/aetherel"
	"github.com/aryshq/aetherel/config"
	"github.com/aryshq/aetherel/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/vearutop/statigz"
)

type Server struct {
	Echo   *echo.Echo
	Logger *slog.Logger
	Cfg    *config.Config
	DB     *pgxpool.Pool
}

func Initialize(cfg *config.Config) (*Server, error) {
	logger := initializeLogger(cfg)

	e := echo.New()
	e.HideBanner = true
	e.HidePort = true

	e.Use(middleware.RequestID())
	e.Use(middleware.Recover())
	e.Use(middleware.Timeout())
	e.Use(aetherel.InjectContext(cfg))

	e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogStatus:   true,
		LogURI:      true,
		LogError:    true,
		HandleError: true, // forwards error to the global error handler, so it can decide appropriate status code
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
			ctx := c.Request().Context()
			if v.Error == nil {
				logger.LogAttrs(ctx, slog.LevelInfo, c.Request().Method,
					slog.String("uri", v.URI),
					slog.Int("status", v.Status),
				)
			} else {
				logger.LogAttrs(ctx, slog.LevelError, c.Request().Method,
					slog.String("uri", v.URI),
					slog.Int("status", v.Status),
					slog.String("err", v.Error.Error()),
				)
			}
			return nil
		},
	}))

	var db *pgxpool.Pool
	if len(cfg.Database.URL) > 0 {
		var err error
		db, err = postgres.Connect(cfg)
		if err != nil {
			return nil, fmt.Errorf("cannot connect to postgres database: %w", err)
		}
	}

	return &Server{
		Echo:   e,
		Logger: logger,
		Cfg:    cfg,
		DB:     db,
	}, nil
}

func (s *Server) ServeStaticFiles(url string, folder string, fs embed.FS) {
	if s.Cfg.App.Debug {
		s.Echo.Group(url, middleware.StaticWithConfig(middleware.StaticConfig{
			Root: folder,
		}))
	} else {
		path := url + "/*"
		s.Echo.GET(
			path,
			echo.WrapHandler(statigz.FileServer(fs, statigz.EncodeOnInit, statigz.FSPrefix(folder))),
			middleware.Rewrite(map[string]string{path: "/$1"}), // remove the path prefix from the file lookup
		)
	}
}

func (s *Server) StartServer(ctx context.Context) error {
	// Handle OS signals to cancelServer the context
	ctxServer, cancelServer := context.WithCancel(ctx)
	go func() {
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
		<-sigs
		cancelServer()
	}()

	errServer := func() error {
		host := fmt.Sprintf("%v:%v", s.Cfg.App.Host, s.Cfg.App.Port)
		slog.Info("Server started", "url", s.Cfg.BaseURL(), "host", host)
		err := s.Echo.Start(host)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}()

	<-ctxServer.Done()

	slog.Info("Shutting down gracefully...")
	ctxShutdown, cancelShutdown := context.WithTimeout(ctx, 5*time.Second)
	defer cancelShutdown()

	if s.DB != nil {
		s.DB.Close()
	}

	err := s.Echo.Shutdown(ctxShutdown)
	if err != nil {
		slog.Error("Could not shut down gracefully", "error", err)
		return fmt.Errorf(
			"could not shut down gracefully: %w. After server error: %v",
			err,
			errServer,
		)
	}

	slog.Info("Server shut down gracefully")
	return nil
}

func initializeLogger(cfg *config.Config) *slog.Logger {
	loggerOptions := &slog.HandlerOptions{
		Level:     cfg.Log.Level.ToSlog(),
		AddSource: cfg.Log.Verbose && cfg.App.Debug,
	}
	var logger *slog.Logger
	switch cfg.Log.Format {
	case config.LogFormatPlaintext:
		{
			logger = slog.New(slog.NewTextHandler(os.Stdout, loggerOptions))
		}
	case config.LogFormatJSON:
		{
			logger = slog.New(slog.NewJSONHandler(os.Stdout, loggerOptions))
		}
	}
	slog.SetDefault(logger)
	return logger
}
