package gumi

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/snowflake/v2"
)

type Source int

const (
	SourceMessage Source = iota
	SourceInteraction
)

func (s Source) String() string {
	switch s {
	case SourceMessage:
		return "message"
	case SourceInteraction:
		return "interaction"
	}
	return "unknown"
}

// Context is a single command invocation. Replies work the same whether
// it came from a slash command or a message. Exactly one of Message and
// Interaction is set.
type Context struct {
	Client  *bot.Client
	Router  *Router
	Command *Command
	Source  Source
	Prefix  string
	Message *discord.Message
	// A discord.ApplicationCommandInteraction, or the
	// discord.AutocompleteInteraction while checks gate suggestions.
	Interaction   discord.Interaction
	Options       *Options
	TargetMessage *discord.Message
	TargetUser    *discord.User
	// Guilds only; nil in DMs.
	TargetMember *discord.Member
	StartedAt    time.Time

	ctx       context.Context
	responder events.InteractionResponderFunc
	mu        sync.Mutex
	values    map[string]any

	responded   bool
	deferred    bool
	edited      bool
	ephemeral   bool
	lastMessage *discord.Message
}

func newContext(r *Router, c *bot.Client, cmd *Command) *Context {
	base := context.Background()
	if r.cfg.BaseContext != nil {
		base = r.cfg.BaseContext()
	}
	return &Context{
		Client:    c,
		Router:    r,
		Command:   cmd,
		Options:   newOptions(),
		StartedAt: time.Now(),
		ctx:       base,
		ephemeral: cmd != nil && cmd.Ephemeral,
	}
}

// Context carries cancellation and deadlines; see middleware.Timeout.
func (c *Context) Context() context.Context {
	if c.ctx == nil {
		return context.Background()
	}
	return c.ctx
}

func (c *Context) SetContext(ctx context.Context) { c.ctx = ctx }

// Set stashes a value for middleware and handlers to share.
func (c *Context) Set(key string, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.values == nil {
		c.values = make(map[string]any)
	}

	c.values[key] = value
}

func (c *Context) Get(key string) (any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.values[key]
	return v, ok
}

func (c *Context) IsInteraction() bool { return c.Interaction != nil }

func (c *Context) IsMessage() bool { return c.Message != nil }

// 0 outside guilds.
func (c *Context) GuildID() snowflake.ID {
	if c.Interaction != nil {
		return idOrZero(c.Interaction.GuildID())
	}

	if c.Message != nil {
		return idOrZero(c.Message.GuildID)
	}

	return 0
}

func (c *Context) ChannelID() snowflake.ID {
	if c.Interaction != nil {
		return interactionChannelID(c.Interaction)
	}

	if c.Message != nil {
		return c.Message.ChannelID
	}

	return 0
}

func (c *Context) Author() *discord.User {
	if c.Interaction != nil {
		u := c.Interaction.User()
		return &u
	}

	if c.Message != nil {
		return &c.Message.Author
	}

	return nil
}

func (c *Context) AuthorID() snowflake.ID {
	if u := c.Author(); u != nil {
		return u.ID
	}

	return 0
}

// nil in DMs.
func (c *Context) Member() *discord.Member {
	if c.Interaction != nil {
		if m := c.Interaction.Member(); m != nil {
			return &m.Member
		}
		return nil
	}

	if c.Message != nil && c.Message.Member != nil {
		m := *c.Message.Member
		if m.User.ID == 0 {
			m.User = c.Message.Author
		}

		if m.GuildID == 0 {
			m.GuildID = idOrZero(c.Message.GuildID)
		}

		return &m
	}

	return nil
}

func (c *Context) Locale() discord.Locale {
	if c.Interaction != nil {
		return c.Interaction.Locale()
	}

	return ""
}

// Channel fetches the invoking channel (cache first, then REST).
func (c *Context) Channel() (discord.Channel, error) {
	return c.channel(c.ChannelID())
}

func (c *Context) channel(id snowflake.ID) (discord.Channel, error) {
	if c.Client.Caches != nil {
		if ch, ok := c.Client.Caches.Channel(id); ok {
			return ch, nil
		}
	}

	return c.Client.Rest.GetChannel(id)
}

// Guild fetches the invoking guild (cache first, then REST).
func (c *Context) Guild() (discord.Guild, error) {
	id := c.GuildID()
	if id == 0 {
		return discord.Guild{}, errors.New("gumi: not in a guild")
	}

	if c.Client.Caches != nil {
		if g, ok := c.Client.Caches.Guild(id); ok {
			return g, nil
		}
	}

	g, err := c.Client.Rest.GetGuild(id, false)
	if err != nil {
		return discord.Guild{}, err
	}

	return g.Guild, nil
}

// Permissions are the invoker's channel permissions (all of them outside
// guilds). Over prefix they come from the message's member and the channel
// and role caches.
func (c *Context) Permissions() (discord.Permissions, error) {
	if c.GuildID() == 0 {
		return discord.PermissionsAll, nil
	}

	if c.Interaction != nil {
		if m := c.Interaction.Member(); m != nil {
			return m.Permissions, nil
		}
	}

	// DisGo doesn't cache message authors, so the cache rarely has them.
	if m := c.Member(); m != nil {
		return c.channelPermissions(*m)
	}

	return c.cachedPermissions(c.AuthorID())
}

func (c *Context) BotPermissions() (discord.Permissions, error) {
	if c.GuildID() == 0 {
		return discord.PermissionsAll, nil
	}

	if c.Interaction != nil {
		if p := c.Interaction.AppPermissions(); p != nil {
			return *p, nil
		}
	}

	return c.cachedPermissions(c.Client.ID())
}

func (c *Context) cachedPermissions(userID snowflake.ID) (discord.Permissions, error) {
	caches := c.Client.Caches
	if caches == nil || userID == 0 {
		return 0, errors.New("gumi: permissions need the client's caches")
	}

	member, ok := caches.Member(c.GuildID(), userID)
	if !ok {
		return 0, fmt.Errorf("gumi: member %s is not cached", userID)
	}

	return c.channelPermissions(member)
}

func (c *Context) channelPermissions(member discord.Member) (discord.Permissions, error) {
	caches := c.Client.Caches
	if caches == nil {
		return 0, errors.New("gumi: permissions need the client's caches")
	}

	ch, ok := caches.Channel(c.ChannelID())
	if !ok {
		return 0, fmt.Errorf("gumi: channel %s is not cached", c.ChannelID())
	}

	return caches.MemberPermissionsInChannel(ch, member), nil
}

// DisplayPrefix is the used prefix, falling back to the guild's first one.
func (c *Context) DisplayPrefix() string {
	if c.Prefix != "" {
		return c.Prefix
	}

	if p := c.Router.Prefixes(c.Client, c.GuildID(), c.ChannelID()); len(p) > 0 {
		return p[0]
	}

	return c.Router.mentionPrefix(c.Client)
}

func (c *Context) messageAttachments() []discord.Attachment {
	if c.Message != nil {
		return c.Message.Attachments
	}
	return nil
}

type Response struct {
	Content         string
	Embeds          []discord.Embed
	Components      []discord.LayoutComponent
	Files           []*discord.File
	AllowedMentions *discord.AllowedMentions
	// Invoker-only for interactions; ignored over prefix.
	Ephemeral bool
	TTS       bool
	Flags     discord.MessageFlags
}

func Text(content string) *Response { return &Response{Content: content} }

func Textf(format string, args ...any) *Response {
	return &Response{Content: fmt.Sprintf(format, args...)}
}

func Embed(embed discord.Embed) *Response {
	return &Response{Embeds: []discord.Embed{embed}}
}

func (r *Response) Private() *Response { r.Ephemeral = true; return r }

func (r *Response) flags(ephemeral bool) discord.MessageFlags {
	f := r.Flags
	if ephemeral || r.Ephemeral {
		f = f.Add(discord.MessageFlagEphemeral)
	}
	return f
}

func (r *Response) messageCreate(ephemeral bool) discord.MessageCreate {
	return discord.MessageCreate{
		Content:         r.Content,
		TTS:             r.TTS,
		Embeds:          r.Embeds,
		Components:      r.Components,
		Files:           r.Files,
		AllowedMentions: r.AllowedMentions,
		Flags:           r.flags(ephemeral),
	}
}

// messageUpdate is the edit payload. Unset embeds and components become
// empty so the edit clears them instead of keeping the old ones.
func (r *Response) messageUpdate() discord.MessageUpdate {
	content := r.Content
	embeds := r.Embeds
	if embeds == nil {
		embeds = []discord.Embed{}
	}

	components := r.Components
	if components == nil {
		components = []discord.LayoutComponent{}
	}

	return discord.MessageUpdate{
		Content:         &content,
		Embeds:          &embeds,
		Components:      &components,
		Files:           r.Files,
		AllowedMentions: r.AllowedMentions,
	}
}

func (r *Response) channelMessage(ref *discord.MessageReference, defaultMentions *discord.AllowedMentions) discord.MessageCreate {
	m := r.messageCreate(false)
	m.Flags = m.Flags.Remove(discord.MessageFlagEphemeral)
	m.MessageReference = ref
	if m.AllowedMentions == nil {
		m.AllowedMentions = defaultMentions
	}
	return m
}

// Reply answers once over interactions (editing the deferred placeholder
// if there is one, else following up), or replies to the message.
func (c *Context) Reply(r *Response) error {
	if r == nil {
		r = &Response{}
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.Interaction != nil {
		switch {
		case !c.responded:
			err := c.responder(discord.InteractionResponseTypeCreateMessage, r.messageCreate(c.ephemeral))
			if err == nil {
				c.responded = true
			}

			return err
		case c.deferred && !c.edited:
			msg, err := c.Client.Rest.UpdateInteractionResponse(c.Interaction.ApplicationID(), c.Interaction.Token(), r.messageUpdate())
			if err == nil {
				c.edited = true
				c.lastMessage = msg
			}

			return err
		default:
			msg, err := c.Client.Rest.CreateFollowupMessage(c.Interaction.ApplicationID(), c.Interaction.Token(), r.messageCreate(c.ephemeral))
			if err == nil {
				c.lastMessage = msg
			}

			return err
		}
	}

	var ref *discord.MessageReference
	if c.Message != nil && !c.Router.cfg.DisableReplyReference {
		ref = softReference(c.Message)
	}

	msg, err := c.Client.Rest.CreateMessage(c.ChannelID(), r.channelMessage(ref, c.Router.cfg.AllowedMentions))
	if err == nil {
		c.responded = true
		c.lastMessage = msg
	}

	return err
}

// Quotes m without failing the reply if m is gone.
func softReference(m *discord.Message) *discord.MessageReference {
	id, channelID := m.ID, m.ChannelID
	return &discord.MessageReference{MessageID: &id, ChannelID: &channelID, GuildID: m.GuildID}
}

func (c *Context) ReplyText(content string) error { return c.Reply(Text(content)) }

func (c *Context) Replyf(format string, args ...any) error {
	return c.Reply(Textf(format, args...))
}

func (c *Context) ReplyEmbed(embed discord.Embed) error { return c.Reply(Embed(embed)) }

func (c *Context) ReplyEphemeral(content string) error { return c.Reply(Text(content).Private()) }

// SetEphemeral decides at runtime whether replies are invoker-only, e.g.
// from an option. Discord fixes it with the first response (including
// Defer), so call it before that. No effect over prefix.
func (c *Context) SetEphemeral(on bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.ephemeral = on
}

// Defer gives the handler up to 15 minutes by showing "thinking…" (or
// typing, over prefix). Calling it twice does nothing.
func (c *Context) Defer() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.Interaction != nil {
		if c.responded {
			return nil
		}

		var data discord.InteractionResponseData
		if c.ephemeral {
			data = discord.MessageCreate{Flags: discord.MessageFlagEphemeral}
		}

		err := c.responder(discord.InteractionResponseTypeDeferredCreateMessage, data)
		if err == nil {
			c.responded, c.deferred = true, true
		}

		return err
	}

	if c.Message != nil {
		return c.Client.Rest.SendTyping(c.ChannelID())
	}

	return nil
}

// Edit rewrites the original response, or the last reply over prefix.
// Replies instead if nothing was sent yet.
func (c *Context) Edit(r *Response) error {
	if r == nil {
		r = &Response{}
	}

	if !c.Responded() {
		return c.Reply(r)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.Interaction != nil {
		msg, err := c.Client.Rest.UpdateInteractionResponse(c.Interaction.ApplicationID(), c.Interaction.Token(), r.messageUpdate())
		if err == nil {
			c.edited = true
			c.lastMessage = msg
		}

		return err
	}

	if c.lastMessage == nil {
		return ErrNoResponse
	}

	msg, err := c.Client.Rest.UpdateMessage(c.lastMessage.ChannelID, c.lastMessage.ID, r.messageUpdate())
	if err == nil {
		c.lastMessage = msg
	}

	return err
}

// Followup sends an extra message, responding first if it has to.
func (c *Context) Followup(r *Response) (*discord.Message, error) {
	if r == nil {
		r = &Response{}
	}

	if c.Interaction != nil {
		if !c.Responded() {
			if err := c.Reply(r); err != nil {
				return nil, err
			}

			return c.OriginalResponse()
		}

		return c.Client.Rest.CreateFollowupMessage(c.Interaction.ApplicationID(), c.Interaction.Token(), r.messageCreate(c.ephemeral))
	}

	return c.Client.Rest.CreateMessage(c.ChannelID(), r.channelMessage(nil, c.Router.cfg.AllowedMentions))
}

func (c *Context) OriginalResponse() (*discord.Message, error) {
	if c.Interaction != nil {
		return c.Client.Rest.GetInteractionResponse(c.Interaction.ApplicationID(), c.Interaction.Token())
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.lastMessage == nil {
		return nil, ErrNoResponse
	}

	return c.lastMessage, nil
}

// Delete removes the original response (interactions) or the last reply.
func (c *Context) Delete() error {
	if c.Interaction != nil {
		return c.Client.Rest.DeleteInteractionResponse(c.Interaction.ApplicationID(), c.Interaction.Token())
	}

	c.mu.Lock()
	msg := c.lastMessage
	c.mu.Unlock()

	if msg == nil {
		return ErrNoResponse
	}

	return c.Client.Rest.DeleteMessage(msg.ChannelID, msg.ID)
}

// Responded reports whether an initial response (or Defer) has been sent.
func (c *Context) Responded() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.responded
}

// ID panics on an interaction without a channel object.
func interactionChannelID(i discord.Interaction) snowflake.ID {
	if ch := i.Channel(); ch.MessageChannel != nil {
		return ch.ID()
	}
	return 0
}
