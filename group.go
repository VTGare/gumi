package gumi

import "time"

type Group struct {
	Name        string
	Description string
	NSFW        bool
	Commands    map[string]*Command
	IsVisible   bool
}

type GroupOption func(*Group)

func (g *Group) SetNSFW(v bool) *Group {
	g.NSFW = v
	return g
}

func (g *Group) SetDescription(s string) *Group {
	g.Description = s
	return g
}

func (g *Group) AddCommand(command *Command) *Command {
	if g.NSFW {
		command.NSFW = true
	}

	if command.Cooldown != 0 {
		command.execMap = make(map[string]time.Time)
	}

	g.Commands[command.Name] = command
	for _, alias := range command.Aliases {
		g.Commands[alias] = command
	}

	return command
}
