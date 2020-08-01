package gumi

//Option is a type alias for optional functions
type Option func(*Gumi)

func WithPrefixes(prefixes ...string) Option {
	return func(g *Gumi) {
		g.DefaultPrefixes = prefixes
	}
}

func WithHelpHandler(hh HelpHandler) Option {
	return func(g *Gumi) {
		g.HelpCommand = hh
	}
}

func WithErrorHandler(eh ErrorHandler) Option {
	return func(g *Gumi) {
		g.ErrorHandler = eh
	}
}

func WithPrefixResolver(pr PrefixResolver) Option {
	return func(g *Gumi) {
		g.PrefixHandler = pr
	}
}
