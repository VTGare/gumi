package gumi

import (
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"
)

type GumiCommand struct {
	Name        string
	Aliases     []string
	Description string
	GuildOnly   bool
	NSFW        bool
	Exec        GumiExec
	Help        *HelpSettings
}

type GumiExec func(*discordgo.Session, *discordgo.MessageCreate, []string) error

type CommandOption func(*GumiCommand)

func CommandDescription(desc string) CommandOption {
	return func(c *GumiCommand) {
		c.Description = desc
	}
}

func GuildOnly() CommandOption {
	return func(c *GumiCommand) {
		c.GuildOnly = true
	}
}

func WithHelp(hs *HelpSettings) CommandOption {
	return func(g *GumiCommand) {
		g.Help = hs
	}
}

//HelpSettings are settings needed for default help command.
type HelpSettings struct {
	IsVisible    bool
	ExtendedHelp []*discordgo.MessageEmbedField
}

func NewHelpSettings() *HelpSettings {
	return &HelpSettings{
		IsVisible:    true,
		ExtendedHelp: make([]*discordgo.MessageEmbedField, 0),
	}
}

func (hs *HelpSettings) AddField(name, value string, inline bool) *HelpSettings {
	field := &discordgo.MessageEmbedField{
		Name:   name,
		Value:  value,
		Inline: inline,
	}

	hs.ExtendedHelp = append(hs.ExtendedHelp, field)
	return hs
}

func NewCommand(name string, exec GumiExec, opts ...CommandOption) *GumiCommand {
	command := &GumiCommand{
		Name:        name,
		Exec:        exec,
		Aliases:     make([]string, 0),
		Description: "",
		GuildOnly:   false,
		NSFW:        false,
		Help:        NewHelpSettings(),
	}

	for _, opt := range opts {
		opt(command)
	}

	return command
}

func (c *GumiCommand) createHelp() string {
	str := ""
	if len(c.Aliases) != 0 {
		str += fmt.Sprintf("**Aliases:** %v\n", strings.Join(c.Aliases, ", "))
	}
	str += c.Description

	return str
}
