package gumi

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/VTGare/gumi/utils"
	"github.com/bwmarrin/discordgo"
)

//ErrorHandler is a function that handles all errors caused by user commands, if nil uses default.
type ErrorHandler func(e error) *discordgo.MessageSend

//HelpHandler ...
type HelpHandler func(*Gumi, *discordgo.Session, *discordgo.MessageCreate, []string) *discordgo.MessageSend

//PrefixResolver ...
type PrefixResolver func() []string

//Gumi is a command framework for DiscordGo
type Gumi struct {
	Groups map[string]*GumiGroup
	//UngroupedCommands map[string]*GumiCommand
	DefaultPrefixes []string
	HelpCommand     HelpHandler
	ErrorHandler    ErrorHandler
	PrefixHandler   PrefixResolver
}

func defaultHelp(g *Gumi, s *discordgo.Session, m *discordgo.MessageCreate, args []string) *discordgo.MessageSend {
	prefix := g.PrefixHandler()

	embed := &discordgo.MessageEmbed{
		Description: fmt.Sprintf("Use ``%vhelp <group name> <command name>`` for extended help on specific commands.", prefix[0]),
		Color:       utils.EmbedColor,
		Timestamp:   utils.EmbedTimestamp(),
		Thumbnail: &discordgo.MessageEmbedThumbnail{
			URL: utils.EmbedImage,
		},
	}

	switch len(args) {
	case 0:
		embed.Title = "Help"
		for _, group := range g.Groups {
			if group.IsVisible {
				field := &discordgo.MessageEmbedField{
					Name:  group.Name,
					Value: group.Description,
				}
				embed.Fields = append(embed.Fields, field)
			}
		}
	case 1:
		if group, ok := g.Groups[args[0]]; ok {
			embed.Title = fmt.Sprintf("%v group command list", args[0])

			used := map[string]bool{}
			for _, command := range group.Commands {
				_, ok := used[command.Name]
				if command.Help.IsVisible && !ok {
					field := &discordgo.MessageEmbedField{
						Name:  command.Name,
						Value: command.createHelp(),
					}
					used[command.Name] = true
					embed.Fields = append(embed.Fields, field)
				}
			}
		} else {
			return g.ErrorHandler(fmt.Errorf("unknown group %v", args[0]))
		}
	case 2:
		if group, ok := g.Groups[args[0]]; ok {
			if command, ok := group.Commands[args[1]]; ok {
				if command.Help.IsVisible && command.Help.ExtendedHelp != nil {
					embed.Title = fmt.Sprintf("%v command extended help", command.Name)
					embed.Fields = command.Help.ExtendedHelp
				} else {
					return g.ErrorHandler(fmt.Errorf("command %v is invisible or doesn't have extended help", args[0]))
				}
			} else {
				return g.ErrorHandler(fmt.Errorf("unknown command %v", args[1]))
			}
		} else {
			return g.ErrorHandler(fmt.Errorf("unknown group %v", args[0]))
		}
	default:
		return g.ErrorHandler(errors.New("incorrect command usage. Example: bt!help <group> <command name>"))
	}

	return &discordgo.MessageSend{
		Embed: embed,
	}
}

func defaultError(e error) *discordgo.MessageSend {
	embed := &discordgo.MessageEmbed{
		Title: "Oops, something went wrong!",
		Thumbnail: &discordgo.MessageEmbedThumbnail{
			URL: utils.EmbedImage,
		},
		Description: fmt.Sprintf("***Error message:***\n%v", e),
		Color:       utils.EmbedColor,
		Timestamp:   utils.EmbedTimestamp(),
	}

	return &discordgo.MessageSend{
		Embed: embed,
	}
}

//NewGumi creates a new Gumi instance
func NewGumi(opts ...Option) *Gumi {
	var (
		defaultPrefix        = []string{"?"}
		defaultPrefixHandler = func() []string {
			return utils.MapString(defaultPrefix, func(s string) string {
				return strings.ToLower(s)
			})
		}
	)

	g := &Gumi{
		DefaultPrefixes: defaultPrefix,
		HelpCommand:     defaultHelp,
		ErrorHandler:    defaultError,
		PrefixHandler:   defaultPrefixHandler,
	}

	for _, opt := range opts {
		opt(g)
	}

	return g
}

//Handle ...
func (g *Gumi) Handle(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.Bot {
		return
	}

	var (
		content = strings.ToLower(m.Content)
		isGuild = m.GuildID != ""
	)

	channel, err := s.Channel(m.ChannelID)
	if err != nil {
		//reserved for logrus
	}

	if content, isCommand := g.trimPrefix(content); isCommand {
		fields := strings.Fields(content)
		if len(fields) == 0 {
			return
		}
		command := fields[0]
		args := fields[1:]

		for _, group := range g.Groups {
			if cmd, ok := group.Commands[command]; ok {
				if cmd.GuildOnly && !isGuild {
					g.ErrorHandler(errNotGuild(cmd.Name))
					return
				}

				if cmd.NSFW && !channel.NSFW {
					prompt := utils.CreatePrompt(s, m, utils.NewPromptOptions("You're trying to execute a NSFW command in SFW channel, are you sure about that?", 15*time.Second))
					if !prompt {
						return
					}
				}

				go func() {
					err := cmd.Exec(s, m, args)
					if errorMessage := g.ErrorHandler(err); errorMessage != nil {
						_, err := s.ChannelMessageSendComplex(m.ChannelID, errorMessage)
						if err != nil {
							//reserved for logrus warn log
						}
					}
				}()
			}
		}
	}
}

func (g *Gumi) trimPrefix(content string) (string, bool) {
	prefixes := g.PrefixHandler()
	trimmed := false

	for _, prefix := range prefixes {
		if strings.HasPrefix(content, prefix) {
			trimmed = true
			return strings.TrimPrefix(content, prefix), trimmed
		}
	}

	return content, trimmed
}

func (g *Gumi) AddGroup(name string, opts ...GroupOption) *GumiGroup {
	group := newGroup(name, opts...)
	g.Groups[name] = group

	return group
}
