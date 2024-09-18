package aetherel

import (
	"context"

	"github.com/aryshq/aetherel/config"
	"github.com/labstack/echo/v4"
)

type ContextKey uint

const (
	ctxConfig ContextKey = iota
)

// Currying function that returns a middleware function that will inject stuff into the request's context.
// The currying is used to inject dependencies into the middleware function while still complying with the standard
// go middleware signature. This middleware will store and retrieve session data and application settings into the
// context of every request.
func InjectContext(
	cfg *config.Config,
) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := c.Request().Context()

			ctx = context.WithValue(ctx, ctxConfig, cfg)

			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	}
}

// Return a copy of the current configuration.
func Config(ctx context.Context) config.Config {
	return *ctx.Value(ctxConfig).(*config.Config)
}
