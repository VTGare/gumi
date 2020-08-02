package gumi

type Group struct {
	Name        string
	Description string
	NSFW        bool
	Commands    map[string]*Command
	IsVisible   bool
}

type GroupOption func(*Group)

func GroupNSFW() GroupOption {
	return func(g *Group) {
		g.NSFW = true
	}
}

func GroupDescription(desc string) GroupOption {
	return func(g *Group) {
		g.Description = desc
	}
}

func newGroup(name string, opts ...GroupOption) *Group {
	g := &Group{
		Name:        name,
		Commands:    make(map[string]*Command),
		Description: "",
		NSFW:        false,
		IsVisible:   true,
	}

	for _, opt := range opts {
		opt(g)
	}

	return g
}

func (g *Group) AddCommand(name string, exec GumiExec, opts ...CommandOption) *Command {
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
