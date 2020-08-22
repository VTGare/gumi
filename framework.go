package gumi

import (
	"strings"
	"time"

	"github.com/VTGare/gumi/utils"
	"github.com/bwmarrin/discordgo"
	"github.com/sirupsen/logrus"
)

//ErrorHandler is a function that handles all errors caused by user commands, if nil uses default.
type ErrorHandler func(e error) *discordgo.MessageSend

//HelpHandler ...
type HelpHandler func(*Gumi, *discordgo.Session, *discordgo.MessageCreate, []string) *discordgo.MessageSend

//PrefixResolver ...
type PrefixResolver func(*Gumi, *discordgo.Session, *discordgo.MessageCreate) []string

//Gumi is a command framework for DiscordGo
type Gumi struct {
	Groups map[string]*Group
	//UngroupedCommands map[string]*GumiCommand
	DefaultPrefixes []string
	HelpCommand     HelpHandler
	ErrorHandler    ErrorHandler
	PrefixHandler   PrefixResolver
}

//NewGumi creates a new Gumi instance
func NewGumi(opts ...Option) *Gumi {
	var (
		defaultPrefix        = []string{"?"}
		defaultPrefixHandler = func(g *Gumi, s *discordgo.Session, m *discordgo.MessageCreate) []string {
			return utils.MapString(g.DefaultPrefixes, func(s string) string {
				return strings.ToLower(s)
			})
		}
	)

	g := &Gumi{
		Groups:          make(map[string]*Group),
		DefaultPrefixes: defaultPrefix,
		HelpCommand:     defaultHelp,
		ErrorHandler:    defaultError,
		PrefixHandler:   defaultPrefixHandler,
	}

	for _, opt := range opts {
		opt(g)
	}

	if g.HelpCommand != nil {
		general := g.AddGroup("general", GroupDescription("General purpose commands"))
		general.AddCommand("help", func(s *discordgo.Session, m *discordgo.MessageCreate, args []string) error {
			help := g.HelpCommand(g, s, m, args)
			_, err := s.ChannelMessageSendComplex(m.ChannelID, help)
			return err
		}, CommandDescription("Sends this message."))
	}

	return g
}

//Handle invokes a command handler for Gumi instance, should be called from within MessageCreate event in discordgo application. Returns true if handled a valid command
func (g *Gumi) Handle(s *discordgo.Session, m *discordgo.MessageCreate) bool {
	if m.Author.Bot {
		return false
	}

	var (
		content = strings.ToLower(m.Content)
		isGuild = m.GuildID != ""
	)

	channel, err := s.Channel(m.ChannelID)
	if err != nil {
		//reserved for logrus
	}

	if content, isCommand := g.trimPrefix(s, m, content); isCommand {
		fields := strings.Fields(content)
		if len(fields) == 0 {
			return false
		}
		command := fields[0]
		args := fields[1:]

		for _, group := range g.Groups {
			if cmd, ok := group.Commands[command]; ok {
				if cmd.GuildOnly && !isGuild {
					g.ErrorHandler(errNotGuild(cmd.Name))
					return true
				}

				if cmd.NSFW && !channel.NSFW {
					prompt := utils.CreatePrompt(s, m, utils.NewPromptOptions("You're trying to execute a NSFW command in SFW channel, are you sure about that?", 15*time.Second))
					if !prompt {
						return true
					}
				}

				go func() {
					logrus.Infof("Executing command: %s. Arguments: %v", cmd.Name, args)
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

		return true
	}

	return false
}

func (g *Gumi) trimPrefix(s *discordgo.Session, m *discordgo.MessageCreate, content string) (string, bool) {
	prefixes := g.PrefixHandler(g, s, m)
	trimmed := false

	for _, prefix := range prefixes {
		if strings.HasPrefix(content, prefix) {
			trimmed = true
			return strings.TrimPrefix(content, prefix), trimmed
		}
	}

	return content, trimmed
}

//AddGroup creates a new group with given name and parameters. Please don't directly modify it, use functions instead.
func (g *Gumi) AddGroup(name string, opts ...GroupOption) *Group {
	group := newGroup(name, opts...)
	g.Groups[name] = group

	return group
}

//SetPrefixHandler sets a PrefixHandler for GumiInstance
func (g *Gumi) SetPrefixHandler(ph PrefixResolver) *Gumi {
	g.PrefixHandler = ph
	return g
}

//SetErrorHandler sets an ErrorHandler for Gumi instance
func (g *Gumi) SetErrorHandler(eh ErrorHandler) *Gumi {
	g.ErrorHandler = eh
	return g
}

//SetHelpCommand sets a HelpHandler for Gumi instance
func (g *Gumi) SetHelpCommand(hc HelpHandler) *Gumi {
	g.HelpCommand = hc
	return g
}

//GeneralGroup returns a group autogenerated for help command and should be used for general-purpose commands. Although, you can modify it for your needs.
func (g *Gumi) GeneralGroup() *Group {
	return g.Groups["general"]
}
