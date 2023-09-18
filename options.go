package dnscache

import "log/slog"

type Option struct {
	apply func(r *Resolver)
}

func WithRefreshCompletedListener(listener func()) Option {
	return Option{apply: func(r *Resolver) {
		r.onCacheRefreshedFn = listener
	}}
}

func WithCustomIPLookupFunc(fn LookupIPFn) Option {
	return Option{apply: func(r *Resolver) {
		r.lookupIPFn = fn
	}}
}

func WithLogger(logger *slog.Logger) Option {
	return Option{apply: func(r *Resolver) {
		r.logger = logger
	}}
}
