package dnscache

import "log/slog"

type Option struct {
	apply func(r *Resolver)
}

func WithLookupIPFunc(fn LookupIPFn) Option {
	return Option{apply: func(r *Resolver) {
		r.lookupIPFn = fn
	}}
}

func WithLogger(logger *slog.Logger) Option {
	return Option{apply: func(r *Resolver) {
		r.logger = logger
	}}
}
