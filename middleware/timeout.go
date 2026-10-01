package middleware

import (
	"context"
	"time"

	"github.com/VTGare/gumi"
)

// Timeout puts a deadline on ctx.Context(). Pass that context to I/O so
// it gets cancelled. Interaction tokens expire after 15 minutes anyway.
func Timeout(d time.Duration) gumi.Middleware {
	return func(next gumi.Handler) gumi.Handler {
		return func(ctx *gumi.Context) error {
			c, cancel := context.WithTimeout(ctx.Context(), d)
			defer cancel()

			ctx.SetContext(c)
			return next(ctx)
		}
	}
}
