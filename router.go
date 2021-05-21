package gumi

import (
	"fmt"
	"strings"

	"github.com/VTGare/gumi/utils"
	"github.com/bwmarrin/discordgo"
)

type Router struct {
	Commands                map[string]*Command
	CaseSensitive           bool
	AuthorID                string
	Storage                 *Storage
	PrefixResolver          func(*discordgo.Session, *discordgo.MessageCreate) []string
	NotCommandCallback      func(*Ctx) error
	OnErrorCallback         func(*Ctx, error)
	OnRateLimitCallback     func(*Ctx) error
	OnNoPermissionsCallback func(*Ctx) error
	OnNSFWCallback          func(*Ctx) error
	OnExecuteCallback       func(*Ctx) error
	OnPanicCallBack         func(ctx *Ctx, recover interface{})
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
		r.OnErrorCallback = func(ctx *Ctx, err error) {
			ctx.Reply(fmt.Sprintf("An error occurred: %v", err))
		}
	}

	if r.OnPanicCallBack == nil {
		r.OnPanicCallBack = func(ctx *Ctx, rec interface{}) {
			fmt.Println("Recovering from panic. Error: ", rec)
			if err, ok := rec.(error); ok {
				r.OnErrorCallback(ctx, err)
			}
		}
	}

	r.Storage = &Storage{
		innerMap: make(map[string]interface{}),
	}

	return r
}

func (r *Router) Initialize(session *discordgo.Session) {
	session.AddHandler(r.Handler())
}

func (r *Router) Handler() func(*discordgo.Session, *discordgo.MessageCreate) {
	return func(session *discordgo.Session, event *discordgo.MessageCreate) {
		ctx := &Ctx{}
		defer func() {
			if rec := recover(); rec != nil {
				r.OnPanicCallBack(ctx, rec)
			}
		}()

		var (
			message = event.Message
			content = event.Message.Content
		)

		if message.Author.Bot {
			return
		}

		hasPrefix, content := utils.HasPrefixes(content, r.PrefixResolver(session, event), r.CaseSensitive)
		if !hasPrefix {
			if r.NotCommandCallback != nil {
				ctx = &Ctx{
					Session: session,
					Event:   event,
					Router:  r,
				}

				err := r.NotCommandCallback(ctx)
				if err != nil {
					r.OnErrorCallback(ctx, err)
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
			ctx = &Ctx{
				Session: session,
				Event:   event,
				Args:    ParseArguments(content),
				Router:  r,
				Command: cmd,
			}

			err := cmd.execute(ctx)
			if err != nil {
				if r.OnErrorCallback != nil {
					r.OnErrorCallback(ctx, err)
				}

				return
			}
		}
	}
}
