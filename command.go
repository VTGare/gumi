package gumi

import (
	"fmt"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

type Command struct {
	Name        string
	Aliases     []string
	Description string
	GuildOnly   bool
	NSFW        bool
	Exec        GumiExec
	Help        *HelpSettings
	Cooldown    time.Duration
	execMap     map[string]time.Time
}

type GumiExec func(*discordgo.Session, *discordgo.MessageCreate, []string) error

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

func (c *Command) createHelp() string {
	str := ""
	if len(c.Aliases) != 0 {
		str += fmt.Sprintf("**Aliases:** %v\n", strings.Join(c.Aliases, ", "))
	}
	str += c.Description

	return str
}

func (c *Command) onCooldown(id string) time.Duration {
	if t, ok := c.execMap[id]; ok {
		d := t.Sub(time.Now())
		if d < c.Cooldown {
			return c.Cooldown + (1 * d)
		}
		delete(c.execMap, id)
		return 0
	}

	return 0
}
