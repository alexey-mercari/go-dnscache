package dnscache

type Param struct {
	apply func(r *Resolver)
}

func WithRefreshCompletedListener(listener func()) Param {
	return Param{apply: func(r *Resolver) {
		r.onCacheRefreshedFn = listener
	}}
}

func WithCustomIPLookupFunc(fn LookupIPFn) Param {
	return Param{apply: func(r *Resolver) {
		r.ipLookupFn = fn
	}}
}
