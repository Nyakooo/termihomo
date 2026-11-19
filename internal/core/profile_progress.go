package core

import "context"

type profileProgressKey struct{}

// WithProfileProgress reports stages without exposing subscription URLs or contents.
// The callback runs on the operation goroutine and must return promptly.
func WithProfileProgress(ctx context.Context, report func(string)) context.Context {
	return context.WithValue(ctx, profileProgressKey{}, report)
}

func reportProfileProgress(ctx context.Context, stage string) {
	if report, ok := ctx.Value(profileProgressKey{}).(func(string)); ok && ctx.Err() == nil {
		report(stage)
	}
}
