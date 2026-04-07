package tool

import "context"

type RunContext struct {
	SessionPath string
}

type runContextKey struct{}

func WithRunContext(ctx context.Context, value RunContext) context.Context {
	return context.WithValue(ctx, runContextKey{}, value)
}

func CurrentRunContext(ctx context.Context) RunContext {
	value, _ := ctx.Value(runContextKey{}).(RunContext)
	return value
}
