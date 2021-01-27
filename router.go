package gumi

import (
	"strings"

	"github.com/VTGare/gumi/utils"
	"github.com/bwmarrin/discordgo"
)

type Router struct {
	Commands                map[string]*Command
	CaseSensitive           bool
	PrefixResolver          func(*discordgo.Session, *discordgo.MessageCreate) []string
	NotCommandCallback      func(*discordgo.Session, *discordgo.MessageCreate) error
	OnErrorCallback         func(*discordgo.Session, *discordgo.MessageCreate, error)
	OnRateLimitCallback     func(*Ctx) error
	OnNoPermissionsCallback func(*Ctx) error
	OnNSFWCallback          func(*Ctx) error
	OnExecuteCallback       func(*Ctx) error
}

func (r *Router) RegisterCmd(command *Command) {
	name := command.Name

	if !r.CaseSensitive {
		name = strings.ToLower(command.Name)
	}
	r.Commands[name] = command
	for _, alias := range command.Aliases {
		if !r.CaseSensitive {
			alias = strings.ToLower(alias)
		}
		r.Commands[alias] = command
	}
}

func Create(r *Router) *Router {
	if r.Commands == nil {
		r.Commands = make(map[string]*Command)
	}

	if r.PrefixResolver == nil {
		r.PrefixResolver = func(s *discordgo.Session, m *discordgo.MessageCreate) []string {
			return []string{"!"}
		}
	}

	if r.OnErrorCallback == nil {
		r.OnErrorCallback = func(s *discordgo.Session, m *discordgo.MessageCreate, err error) {
			s.ChannelMessageSend(m.ChannelID, "An error occured: "+err.Error())
		}
	}

	return r
}

func (r *Router) Initialize(session *discordgo.Session) {
	session.AddHandler(r.Handler())
}

func (r *Router) Handler() func(*discordgo.Session, *discordgo.MessageCreate) {
	return func(session *discordgo.Session, event *discordgo.MessageCreate) {
		var (
			message = event.Message
			content = event.Message.Content
		)

		if message.Author.Bot {
			return
		}

		hasPrefix, content := utils.HasPrefixes(content, r.PrefixResolver(session, event))
		if !hasPrefix {
			if r.NotCommandCallback != nil {
				err := r.NotCommandCallback(session, event)
				if err != nil {
					r.OnErrorCallback(session, event, err)
				}
			}

			return
		}

		content = strings.Trim(content, " ")
		if content == "" {
			return
		}

		cmdName := strings.Split(content, " ")[0]
		content = strings.TrimPrefix(content, cmdName)
		if !r.CaseSensitive {
			cmdName = strings.ToLower(cmdName)
		}

		if cmd, ok := r.Commands[cmdName]; ok {
			ctx := &Ctx{
				Session: session,
				Event:   event,
				Args:    ParseArguments(content),
				Router:  r,
				Command: cmd,
			}

			err := cmd.execute(ctx)
			if err != nil {
				if r.OnErrorCallback != nil {
					r.OnErrorCallback(session, event, err)
				}

				return
			}
		}
	}
}
