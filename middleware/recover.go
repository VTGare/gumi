package middleware

import (
	"runtime/debug"

	"github.com/VTGare/gumi/v2"
)

// Recover turns panics into *gumi.PanicError, flowing through error
// handling (and middleware registered before it, like Logging).
func Recover() gumi.Middleware {
	return func(next gumi.Handler) gumi.Handler {
		return func(ctx *gumi.Context) (err error) {
			defer func() {
				if rec := recover(); rec != nil {
					err = &gumi.PanicError{Value: rec, Stack: debug.Stack()}
				}
			}()

			return next(ctx)
		}
	}
}
