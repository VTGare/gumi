// Package gumi declares a Discord command once and runs it both as a
// slash (or context menu) command and as a classic prefixed message command.
// Handlers read ctx.Options and never care how they were invoked.
package gumi

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/snowflake/v2"
)

// PrefixResolver returns the prefixes accepted in the given guild/channel.
// guildID is 0 in DMs.
type PrefixResolver func(c *bot.Client, guildID, channelID snowflake.ID) []string

// ErrorHandler receives every error returned from the command pipeline.
type ErrorHandler func(ctx *Context, err error)

// FallbackHandler receives messages (from non-bot users) that were not
// dispatched as a command.
type FallbackHandler func(e *events.MessageCreate)

type Config struct {
	// Prefixes accepted everywhere. Ignored if PrefixResolver is set.
	Prefixes []string

	// PrefixResolver returns per-guild prefixes.
	PrefixResolver PrefixResolver

	// DisableMentionPrefix turns off "@Bot command" invocation.
	DisableMentionPrefix bool

	// DisablePrefixCommands turns off prefix invocation entirely.
	DisablePrefixCommands bool

	// DisableSlashCommands turns off interaction handling and Sync.
	DisableSlashCommands bool

	// DevGuildID registers every slash command to this guild instead of
	// globally (instant propagation, useful for development).
	DevGuildID snowflake.ID

	// For Sync. Defaults to the client's.
	ApplicationID snowflake.ID

	OwnerIDs []snowflake.ID

	// ErrorHandler replaces the default error handler.
	ErrorHandler ErrorHandler

	// AutocompleteErrorHandler receives autocomplete handler errors, which
	// are otherwise dropped.
	AutocompleteErrorHandler AutocompleteErrorHandler

	// Fallback is called for messages that were not commands.
	Fallback FallbackHandler

	// AllowedMentions applied to prefix replies that don't set their own.
	AllowedMentions *discord.AllowedMentions

	// DisableReplyReference stops prefix replies from quoting the invoking
	// message.
	DisableReplyReference bool

	// BaseContext produces the root context.Context for each invocation.
	BaseContext func() context.Context
}

type slashKey struct {
	t    CommandType
	name string
}

type Router struct {
	cfg Config

	mu          sync.RWMutex
	commands    []*Command
	prefixIndex map[string]*Command
	slashIndex  map[slashKey]*Command
	middleware  []Middleware
	checks      []Check
	owners      map[snowflake.ID]struct{}
	// commandIDs are the synced application command IDs, for mentions.
	commandIDs map[slashKey]snowflake.ID
}

func New(cfg Config) *Router {
	r := &Router{
		cfg:         cfg,
		prefixIndex: make(map[string]*Command),
		slashIndex:  make(map[slashKey]*Command),
		owners:      make(map[snowflake.ID]struct{}),
		commandIDs:  make(map[slashKey]snowflake.ID),
	}
	for _, id := range cfg.OwnerIDs {
		r.owners[id] = struct{}{}
	}
	return r
}

func (r *Router) Config() Config { return r.cfg }

func (r *Router) IsOwner(userID snowflake.ID) bool {
	_, ok := r.owners[userID]
	return ok
}

// Use appends router-wide middleware. Middleware runs in registration order.
func (r *Router) Use(mw ...Middleware) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.middleware = append(r.middleware, mw...)
}

// AddCheck appends router-wide checks that run before every command.
func (r *Router) AddCheck(checks ...Check) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.checks = append(r.checks, checks...)
}

// Register validates and adds top-level commands. Either all commands are
// registered or none.
func (r *Router) Register(cmds ...*Command) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	newPrefix := make(map[string]*Command)
	newSlash := make(map[slashKey]*Command)

	for _, c := range cmds {
		if err := c.validate(0); err != nil {
			return err
		}
		c.parent = nil

		if !c.DisablePrefix {
			for _, key := range c.prefixKeys() {
				if _, ok := r.prefixIndex[key]; ok {
					return fmt.Errorf("gumi: prefix name %q is already registered", key)
				}
				if _, ok := newPrefix[key]; ok {
					return fmt.Errorf("gumi: prefix name %q is registered twice", key)
				}
				newPrefix[key] = c
			}
		}
		if !c.DisableSlash {
			key := slashKey{c.Type, c.Name}
			if _, ok := r.slashIndex[key]; ok {
				return fmt.Errorf("gumi: %s command %q is already registered", c.Type, c.Name)
			}
			if _, ok := newSlash[key]; ok {
				return fmt.Errorf("gumi: %s command %q is registered twice", c.Type, c.Name)
			}
			newSlash[key] = c
		}
	}

	maps.Copy(r.prefixIndex, newPrefix)
	maps.Copy(r.slashIndex, newSlash)

	r.commands = append(r.commands, cmds...)
	return nil
}

// MustRegister is Register but panics on error.
func (r *Router) MustRegister(cmds ...*Command) {
	if err := r.Register(cmds...); err != nil {
		panic(err)
	}
}

func (r *Router) Commands() []*Command {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Command, len(r.commands))
	copy(out, r.commands)
	return out
}

// Lookup resolves a command by prefix path, e.g. Lookup("set", "prefix").
// Names and aliases both match; nil if not found.
func (r *Router) Lookup(path ...string) *Command {
	if len(path) == 0 {
		return nil
	}
	r.mu.RLock()
	cmd := r.prefixIndex[strings.ToLower(path[0])]
	r.mu.RUnlock()

	if cmd == nil {
		// Fall back to slash names so Lookup works for DisablePrefix commands.
		for _, c := range r.Commands() {
			if strings.EqualFold(c.Name, path[0]) {
				cmd = c
				break
			}
		}
	}
	for _, p := range path[1:] {
		if cmd == nil {
			return nil
		}
		cmd = cmd.Subcommand(p)
	}
	return cmd
}

func (r *Router) Prefixes(c *bot.Client, guildID, channelID snowflake.ID) []string {
	if r.cfg.PrefixResolver != nil {
		return r.cfg.PrefixResolver(c, guildID, channelID)
	}

	return r.cfg.Prefixes
}

// mentionPrefix is how mention invocation ("@Bot ") reads, or "" when it
// is off or the bot user is unknown.
func (r *Router) mentionPrefix(c *bot.Client) string {
	if r.cfg.DisableMentionPrefix {
		return ""
	}

	self, ok := selfUser(c)
	if !ok {
		return ""
	}

	return "@" + self.Username + " "
}

// The returned function removes the router again.
func (r *Router) Bind(c *bot.Client) func() {
	c.AddEventListeners(r)
	return func() { c.RemoveEventListeners(r) }
}

func (r *Router) OnEvent(event bot.Event) {
	switch e := event.(type) {
	case *events.MessageCreate:
		r.HandleMessage(e)
	case *events.InteractionCreate:
		r.HandleInteraction(e)
	}
}

func (r *Router) HandleMessage(e *events.MessageCreate) {
	if e == nil || e.GenericMessage == nil || e.Message.Author.Bot {
		return
	}
	if !r.dispatchMessage(e.Client(), e.Message) && r.cfg.Fallback != nil {
		r.cfg.Fallback(e)
	}
}

// dispatchMessage returns true if the message was handled as a command.
func (r *Router) dispatchMessage(c *bot.Client, m discord.Message) bool {
	if r.cfg.DisablePrefixCommands {
		return false
	}

	prefix, rest, ok := r.matchPrefix(c, m)
	if !ok {
		return false
	}
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return false
	}

	toks := tokenize(rest)
	r.mu.RLock()
	cmd := r.prefixIndex[strings.ToLower(toks[0].text)]
	r.mu.RUnlock()

	if cmd == nil || cmd.DisablePrefix {
		return false
	}

	var parseErr error
	i := 1
	for cmd.IsGroup() {
		var sub *Command
		if i < len(toks) {
			sub = cmd.Subcommand(toks[i].text)
		}

		if sub == nil && cmd.Default != "" {
			if def := cmd.Subcommand(cmd.Default); !def.DisablePrefix {
				cmd = def
				continue
			}
		}

		if i >= len(toks) {
			parseErr = &SubcommandError{Command: cmd}
			break
		}
		if sub == nil || sub.DisablePrefix {
			parseErr = &SubcommandError{Command: cmd, Given: toks[i].text}
			break
		}
		cmd = sub
		i++
	}

	args := ""
	if i < len(toks) {
		args = rest[toks[i].start:]
	}

	ctx := newContext(r, c, cmd)
	ctx.Source = SourceMessage
	ctx.Prefix = prefix
	ctx.Message = &m

	if parseErr == nil {
		switch cmd.Type {
		case ChatInput:
			ctx.Options, parseErr = parsePrefixOptions(ctx, cmd, args)
		case MessageContext:
			parseErr = resolveMessageTarget(ctx, args)
		case UserContext:
			parseErr = resolveUserTarget(ctx, args)
		}
	}

	r.dispatch(ctx, parseErr)
	return true
}

func (r *Router) matchPrefix(c *bot.Client, m discord.Message) (prefix, rest string, ok bool) {
	content := m.Content

	if !r.cfg.DisableMentionPrefix {
		if self, ok := selfUser(c); ok {
			id := self.ID.String()
			for _, p := range []string{"<@" + id + ">", "<@!" + id + ">"} {
				if strings.HasPrefix(content, p) {
					return p, content[len(p):], true
				}
			}
		}
	}

	// The longest match wins, so "bt!" beats "bt" for "bt!help".
	for _, p := range r.Prefixes(c, idOrZero(m.GuildID), m.ChannelID) {
		if p != "" && len(p) > len(prefix) && len(content) >= len(p) && strings.EqualFold(content[:len(p)], p) {
			prefix = p
		}
	}

	if prefix == "" {
		return "", "", false
	}

	return prefix, content[len(prefix):], true
}

func resolveMessageTarget(ctx *Context, args string) error {
	m := ctx.Message
	if m.ReferencedMessage != nil {
		ctx.TargetMessage = m.ReferencedMessage
		return nil
	}
	if toks := tokenize(args); len(toks) > 0 {
		channelID, messageID, ok := parseMessageRef(toks[0].text, m.ChannelID)
		if !ok {
			return NewUserError("Reply to a message or provide a message link to use this command.")
		}
		msg, err := ctx.Client.Rest.GetMessage(channelID, messageID)
		if err != nil {
			return WrapUserError("I couldn't find that message.", err)
		}
		ctx.TargetMessage = msg
		return nil
	}
	return NewUserError("Reply to a message or provide a message link to use this command.")
}

func resolveUserTarget(ctx *Context, args string) error {
	m := ctx.Message
	toks := tokenize(args)
	if len(toks) == 0 {
		author := m.Author
		ctx.TargetUser = &author
		ctx.TargetMember = ctx.Member()
		return nil
	}

	id := parseMention(toks[0].text, discord.MentionTypeUser)
	if id == 0 {
		return NewUserError("Expected a user mention or ID.")
	}
	v := &Value{Type: OptionUser, id: id, kind: kindUser, client: ctx.Client, guildID: ctx.GuildID()}
	if ctx.GuildID() != 0 {
		if member, err := v.Member(); err == nil {
			ctx.TargetMember = member
			ctx.TargetUser = &member.User
			return nil
		}
	}
	user, err := v.User()
	if err != nil {
		return WrapUserError("I couldn't find that user.", err)
	}
	ctx.TargetUser = user
	return nil
}

// HandleInteraction runs application commands, their autocomplete, and
// the components and modals built with ComponentID. It ignores everything
// else, so it can share a client with your own component and modal
// handlers.
func (r *Router) HandleInteraction(e *events.InteractionCreate) {
	if e == nil || e.Interaction == nil {
		return
	}
	switch i := e.Interaction.(type) {
	case discord.ComponentInteraction, discord.ModalSubmitInteraction:
		r.handleComponent(e.Client(), i, e.Respond)
	case discord.ApplicationCommandInteraction:
		if !r.cfg.DisableSlashCommands {
			r.handleCommand(e.Client(), i, e.Respond)
		}
	case discord.AutocompleteInteraction:
		if !r.cfg.DisableSlashCommands {
			r.handleAutocomplete(e.Client(), i, e.Respond)
		}
	}
}

func resolvePath(cmd *Command, path []string) (*Command, error) {
	for i := 0; cmd.IsGroup(); i++ {
		if i >= len(path) {
			return cmd, &SubcommandError{Command: cmd}
		}
		sub := cmd.Subcommand(path[i])
		if sub == nil {
			return cmd, &SubcommandError{Command: cmd, Given: path[i]}
		}
		cmd = sub
	}
	return cmd, nil
}

func subcommandPath(group, sub *string) []string {
	var path []string
	if group != nil {
		path = append(path, *group)
	}
	if sub != nil {
		path = append(path, *sub)
	}
	return path
}

func (r *Router) handleCommand(c *bot.Client, i discord.ApplicationCommandInteraction, respond events.InteractionResponderFunc) {
	data := i.Data

	r.mu.RLock()
	cmd := r.slashIndex[slashKey{commandTypeOf(data.Type()), data.CommandName()}]
	r.mu.RUnlock()

	if cmd == nil {
		return
	}

	var parseErr error
	slash, isSlash := data.(discord.SlashCommandInteractionData)
	if isSlash {
		cmd, parseErr = resolvePath(cmd, subcommandPath(slash.SubCommandGroupName, slash.SubCommandName))
	}

	ctx := newContext(r, c, cmd)
	ctx.Source = SourceInteraction
	ctx.Interaction = i
	ctx.responder = respond

	if parseErr == nil {
		switch d := data.(type) {
		case discord.SlashCommandInteractionData:
			ctx.Options = optionsFromSlash(c, ctx.GuildID(), d.Options, &d.Resolved)
		case discord.MessageCommandInteractionData:
			if msg, ok := d.Resolved.Messages[d.TargetID()]; ok {
				ctx.TargetMessage = &msg
			} else {
				parseErr = ErrMissingTarget
			}
		case discord.UserCommandInteractionData:
			if user, ok := d.Resolved.Users[d.TargetID()]; ok {
				ctx.TargetUser = &user
				if member, ok := d.Resolved.Members[d.TargetID()]; ok {
					member.User = user
					ctx.TargetMember = &member.Member
				}
			} else {
				parseErr = ErrMissingTarget
			}
		}
	}

	r.dispatch(ctx, parseErr)
}

// dispatch runs middleware, then checks and cooldowns, then the handler.
// Parse errors go through the middleware too, so logging sees them.
func (r *Router) dispatch(ctx *Context, parseErr error) {
	cmd := ctx.Command
	chain := cmd.chain()
	mws, checks := r.pipeline(cmd)

	var h Handler = func(ctx *Context) error {
		if parseErr != nil {
			return parseErr
		}
		for _, chk := range checks {
			if err := chk(ctx); err != nil {
				return err
			}
		}
		for _, c := range chain {
			if c.Cooldown == nil {
				continue
			}
			if remaining, ok := c.Cooldown.Take(c.Cooldown.Key(ctx)); !ok {
				return &CooldownError{Remaining: remaining, Scope: c.Cooldown.Scope}
			}
		}
		if cmd.Defer {
			if err := ctx.Defer(); err != nil {
				return err
			}
		}
		if cmd.Handler == nil {
			return ErrNoHandler
		}
		return cmd.Handler(ctx)
	}

	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}

	if err := h(ctx); err != nil {
		r.handleError(ctx, err)
	}
}

// pipeline collects the router's middleware and checks, then those of
// each command from the root down to cmd.
func (r *Router) pipeline(cmd *Command) (mws []Middleware, checks []Check) {
	r.mu.RLock()
	mws = slices.Clone(r.middleware)
	checks = slices.Clone(r.checks)
	r.mu.RUnlock()

	for _, c := range cmd.chain() {
		mws = append(mws, c.Middleware...)
		checks = append(checks, c.Checks...)
	}

	return mws, checks
}

func (r *Router) handleError(ctx *Context, err error) {
	if r.cfg.ErrorHandler != nil {
		r.cfg.ErrorHandler(ctx, err)
		return
	}

	DefaultErrorHandler(ctx, err)
}

// DefaultErrorHandler replies with the user-facing message, if any, and a
// generic one otherwise. Silent check failures get no reply.
func DefaultErrorHandler(ctx *Context, err error) {
	if ce, ok := errors.AsType[*CheckError](err); ok && ce.Silent {
		return
	}

	if msg, ok := UserMessageOf(err); ok {
		_ = ctx.Reply(Text(msg).Private())
		return
	}
	_ = ctx.Reply(Text("Something went wrong while running this command.").Private())
}

// ApplicationCommands builds the Discord definitions, split into global and
// per-guild commands. DevGuildID is not applied here; see Sync.
func (r *Router) ApplicationCommands() (global []discord.ApplicationCommandCreate, byGuild map[snowflake.ID][]discord.ApplicationCommandCreate) {
	byGuild = make(map[snowflake.ID][]discord.ApplicationCommandCreate)
	for _, c := range r.Commands() {
		if c.DisableSlash {
			continue
		}
		ac := c.applicationCommand()
		if len(c.GuildIDs) == 0 {
			global = append(global, ac)
			continue
		}
		for _, gid := range c.GuildIDs {
			byGuild[gid] = append(byGuild[gid], ac)
		}
	}
	return global, byGuild
}

// Sync bulk-overwrites the application commands. With DevGuildID set,
// everything lands in that guild instead (globals are left alone).
func (r *Router) Sync(c *bot.Client) error {
	if r.cfg.DisableSlashCommands {
		return nil
	}
	appID := r.applicationID(c)

	global, byGuild := r.ApplicationCommands()
	if r.cfg.DevGuildID != 0 {
		byGuild[r.cfg.DevGuildID] = dedupeCommands(append(byGuild[r.cfg.DevGuildID], global...))
	} else {
		synced, err := c.Rest.SetGlobalCommands(appID, nonNil(global))
		if err != nil {
			return fmt.Errorf("gumi: sync global commands: %w", err)
		}
		r.storeCommandIDs(synced)
	}

	for gid, cmds := range byGuild {
		synced, err := c.Rest.SetGuildCommands(appID, gid, nonNil(cmds))
		if err != nil {
			return fmt.Errorf("gumi: sync commands for guild %s: %w", gid, err)
		}
		// Per-guild IDs differ between guilds, so only the dev guild's
		// (which holds everything) are usable for mentions.
		if gid == r.cfg.DevGuildID {
			r.storeCommandIDs(synced)
		}
	}
	return nil
}

func (r *Router) storeCommandIDs(cmds []discord.ApplicationCommand) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range cmds {
		r.commandIDs[slashKey{commandTypeOf(c.Type()), c.Name()}] = c.ID()
	}
}

// Mention renders a chat command as a clickable </name:id> mention, or ""
// before Sync or for commands Discord cannot mention.
func (r *Router) Mention(c *Command) string {
	if c.Type != ChatInput || !slashEnabled(r, c) {
		return ""
	}
	r.mu.RLock()
	id := r.commandIDs[slashKey{ChatInput, c.Root().Name}]
	r.mu.RUnlock()
	if id == 0 {
		return ""
	}
	return discord.SlashCommandMention(id, c.QualifiedName())
}

// ClearCommands wipes global (guildID 0) or guild commands.
func (r *Router) ClearCommands(c *bot.Client, guildID snowflake.ID) error {
	appID := r.applicationID(c)
	var err error
	if guildID == 0 {
		_, err = c.Rest.SetGlobalCommands(appID, []discord.ApplicationCommandCreate{})
	} else {
		_, err = c.Rest.SetGuildCommands(appID, guildID, []discord.ApplicationCommandCreate{})
	}
	return err
}

func (r *Router) applicationID(c *bot.Client) snowflake.ID {
	if r.cfg.ApplicationID != 0 {
		return r.cfg.ApplicationID
	}
	return c.ApplicationID
}

// Discord expects an empty overwrite as [], not null.
func nonNil(cmds []discord.ApplicationCommandCreate) []discord.ApplicationCommandCreate {
	if cmds == nil {
		return []discord.ApplicationCommandCreate{}
	}
	return cmds
}

func dedupeCommands(cmds []discord.ApplicationCommandCreate) []discord.ApplicationCommandCreate {
	type key struct {
		t    discord.ApplicationCommandType
		name string
	}
	seen := make(map[key]bool)
	out := cmds[:0]
	for _, c := range cmds {
		k := key{c.Type(), c.CommandName()}
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, c)
	}
	return out
}

func selfUser(c *bot.Client) (discord.OAuth2User, bool) {
	if c == nil || c.Caches == nil {
		return discord.OAuth2User{}, false
	}
	return c.Caches.SelfUser()
}

func idOrZero(id *snowflake.ID) snowflake.ID {
	if id == nil {
		return 0
	}
	return *id
}
