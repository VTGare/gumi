package gumi

import (
	"strings"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/snowflake/v2"
)

// ComponentHandler handles clicks, selects and modal submits on components
// whose custom IDs came from ComponentID.
type ComponentHandler func(ctx *ComponentContext) error

// componentPrefix namespaces the router's component custom IDs.
const componentPrefix = "rt:"

// maxCustomID is Discord's custom ID limit.
const maxCustomID = 100

// ComponentID builds a custom ID that routes back to cmd's Components
// handler with args. It returns "" when the ID would exceed Discord's
// 100-character limit; args must not contain ':'.
func ComponentID(cmd *Command, args ...string) string {
	id := componentPrefix + cmd.Root().Name
	if len(args) > 0 {
		id += ":" + strings.Join(args, ":")
	}

	if len(id) > maxCustomID {
		return ""
	}

	return id
}

// ComponentContext is a button click, select or modal submit.
type ComponentContext struct {
	Client  *bot.Client
	Router  *Router
	Command *Command
	// A discord.ComponentInteraction or discord.ModalSubmitInteraction.
	Interaction discord.Interaction
	// Args are the custom ID's parts after the command name.
	Args []string

	responder events.InteractionResponderFunc
	responded bool
}

// Arg returns the i-th arg, or "" when absent.
func (c *ComponentContext) Arg(i int) string {
	if i < len(c.Args) {
		return c.Args[i]
	}
	return ""
}

// 0 outside guilds.
func (c *ComponentContext) GuildID() snowflake.ID { return idOrZero(c.Interaction.GuildID()) }

func (c *ComponentContext) ChannelID() snowflake.ID { return interactionChannelID(c.Interaction) }

// UserID returns the ID of the user who clicked.
func (c *ComponentContext) UserID() snowflake.ID { return c.Interaction.User().ID }

// nil in DMs.
func (c *ComponentContext) Member() *discord.Member {
	if m := c.Interaction.Member(); m != nil {
		return &m.Member
	}
	return nil
}

// Permissions are the user's channel permissions (all of them in DMs).
func (c *ComponentContext) Permissions() discord.Permissions {
	if m := c.Interaction.Member(); m != nil {
		return m.Permissions
	}
	return discord.PermissionsAll
}

// IsModal reports whether this is a modal submit.
func (c *ComponentContext) IsModal() bool {
	_, ok := c.Interaction.(discord.ModalSubmitInteraction)
	return ok
}

// Entity selects give their IDs as strings.
func (c *ComponentContext) Values() []string {
	i, ok := c.Interaction.(discord.ComponentInteraction)
	if !ok {
		return nil
	}

	switch d := i.Data.(type) {
	case discord.StringSelectMenuInteractionData:
		return d.Values
	case discord.UserSelectMenuInteractionData:
		return idStrings(d.Values)
	case discord.RoleSelectMenuInteractionData:
		return idStrings(d.Values)
	case discord.MentionableSelectMenuInteractionData:
		return idStrings(d.Values)
	case discord.ChannelSelectMenuInteractionData:
		return idStrings(d.Values)
	}
	return nil
}

func idStrings(ids []snowflake.ID) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = id.String()
	}
	return out
}

// ResolvedChannels are the channels picked in a channel select, by ID.
func (c *ComponentContext) ResolvedChannels() map[snowflake.ID]discord.ResolvedChannel {
	if i, ok := c.Interaction.(discord.ComponentInteraction); ok {
		if d, ok := i.Data.(discord.ChannelSelectMenuInteractionData); ok {
			return d.Resolved.Channels
		}
	}
	return nil
}

// TextInput is a modal text input's submitted value, or "".
func (c *ComponentContext) TextInput(customID string) string {
	if i, ok := c.Interaction.(discord.ModalSubmitInteraction); ok {
		return i.Data.Text(customID)
	}
	return ""
}

// Update edits the message the component is on.
func (c *ComponentContext) Update(r *Response) error {
	return c.respond(discord.InteractionResponseTypeUpdateMessage, r.messageUpdate())
}

// Reply sends a new message; set Ephemeral for an invoker-only one.
func (c *ComponentContext) Reply(r *Response) error {
	return c.respond(discord.InteractionResponseTypeCreateMessage, r.messageCreate(false))
}

// Modal opens a modal whose submit routes back with the given custom ID.
func (c *ComponentContext) Modal(customID, title string, components ...discord.LayoutComponent) error {
	return c.respond(discord.InteractionResponseTypeModal, discord.ModalCreate{
		CustomID:   customID,
		Title:      title,
		Components: components,
	})
}

func (c *ComponentContext) respond(t discord.InteractionResponseType, data discord.InteractionResponseData) error {
	if c.responded {
		return ErrAlreadyResponded
	}

	err := c.responder(t, data)
	if err == nil {
		c.responded = true
	}
	return err
}

func componentCustomID(i discord.Interaction) string {
	switch i := i.(type) {
	case discord.ModalSubmitInteraction:
		return i.Data.CustomID
	case discord.ComponentInteraction:
		return i.Data.CustomID()
	}
	return ""
}

// handleComponent routes a component or modal submit to the root command
// named in its custom ID. Components skip middleware, so panics are
// recovered here and errors answered privately.
func (r *Router) handleComponent(c *bot.Client, i discord.Interaction, respond events.InteractionResponderFunc) {
	rest, ok := strings.CutPrefix(componentCustomID(i), componentPrefix)
	if !ok {
		return
	}
	name, args, _ := strings.Cut(rest, ":")

	var cmd *Command
	for _, c := range r.Commands() {
		if c.Name == name && c.Components != nil {
			cmd = c
			break
		}
	}
	if cmd == nil {
		return
	}

	ctx := &ComponentContext{Client: c, Router: r, Command: cmd, Interaction: i, responder: respond}
	if args != "" {
		ctx.Args = strings.Split(args, ":")
	}

	defer func() { _ = recover() }()
	if err := cmd.Components(ctx); err != nil && !ctx.responded {
		msg, ok := UserMessageOf(err)
		if !ok {
			msg = "Something went wrong."
		}
		_ = ctx.Reply(Text(msg).Private())
	}
}
