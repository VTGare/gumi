package gumi

type GumiGroup struct {
	Name        string
	Description string
	NSFW        bool
	Commands    map[string]*GumiCommand
	IsVisible   bool
}

type GroupOption func(*GumiGroup)

func GroupNSFW() GroupOption {
	return func(g *GumiGroup) {
		g.NSFW = true
	}
}

func GroupDescription(desc string) GroupOption {
	return func(g *GumiGroup) {
		g.Description = desc
	}
}

func newGroup(name string, opts ...GroupOption) *GumiGroup {
	g := &GumiGroup{
		Name:        name,
		Description: "",
		NSFW:        false,
		IsVisible:   true,
	}

	for _, opt := range opts {
		opt(g)
	}

	return g
}

func (g *GumiGroup) AddCommand(name string, exec GumiExec, opts ...CommandOption) *GumiCommand {
	command := NewCommand(name, exec, opts...)
	if g.NSFW {
		command.NSFW = g.NSFW
	}

	g.Commands[name] = command
	for _, alias := range command.Aliases {
		g.Commands[alias] = command
	}

	return command
}
