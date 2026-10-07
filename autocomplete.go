package gumi

import (
	"context"
	"fmt"
	"runtime/debug"
	"time"
	"unicode/utf8"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/snowflake/v2"
)

// AutocompleteHandler suggests values for the option being typed. At most
// 25 choices are shown; names over 100 characters are trimmed and string
// values over 100 characters dropped.
type AutocompleteHandler func(ctx *AutocompleteContext) ([]Choice, error)

// AutocompleteErrorHandler receives errors and recovered panics from
// autocomplete handlers. The user just sees no suggestions.
type AutocompleteErrorHandler func(ctx *AutocompleteContext, err error)

// autocompleteTimeout leaves headroom under Discord's 3 second deadline.
const autocompleteTimeout = 2500 * time.Millisecond

// AutocompleteContext is one keystroke's worth of an option being typed.
type AutocompleteContext struct {
	Client      *bot.Client
	Router      *Router
	Command     *Command
	Interaction discord.AutocompleteInteraction
	// Option is the declaration of the focused option.
	Option *Option
	// Value is what has been typed into the focused option so far.
	Value string
	// Options holds every option filled in so far, the focused one included.
	Options *Options

	ctx context.Context
}

// Context expires shortly before Discord stops waiting for suggestions.
func (c *AutocompleteContext) Context() context.Context { return c.ctx }

// 0 outside guilds.
func (c *AutocompleteContext) GuildID() snowflake.ID { return idOrZero(c.Interaction.GuildID()) }

func (c *AutocompleteContext) ChannelID() snowflake.ID { return interactionChannelID(c.Interaction) }

// UserID returns the ID of the user who is typing.
func (c *AutocompleteContext) UserID() snowflake.ID { return c.Interaction.User().ID }

func (c *AutocompleteContext) Locale() discord.Locale { return c.Interaction.Locale() }

// handleAutocomplete answers with the focused option's suggestions. The
// command's checks run first, so users who couldn't run the command get
// no suggestions; middleware and cooldowns don't run.
func (r *Router) handleAutocomplete(c *bot.Client, i discord.AutocompleteInteraction, respond events.InteractionResponderFunc) {
	data := i.Data

	r.mu.RLock()
	cmd := r.slashIndex[slashKey{ChatInput, data.CommandName}]
	r.mu.RUnlock()

	if cmd == nil {
		return
	}

	cmd, err := resolvePath(cmd, subcommandPath(data.SubCommandGroupName, data.SubCommandName))
	if err != nil {
		return
	}

	focused, ok := data.Find(func(o discord.AutocompleteOption) bool { return o.Focused })
	if !ok {
		return
	}

	var decl *Option
	for _, o := range cmd.Options {
		if o.Name == focused.Name && o.Autocomplete != nil {
			decl = o
			break
		}
	}
	if decl == nil {
		return
	}

	base := context.Background()
	if r.cfg.BaseContext != nil {
		base = r.cfg.BaseContext()
	}
	tctx, cancel := context.WithTimeout(base, autocompleteTimeout)
	defer cancel()

	guildID := idOrZero(i.GuildID())
	options := newOptions()
	for name, o := range data.Options {
		options.set(name, interactionValue(c, guildID, o.Type, o.Value, nil))
	}

	ctx := &AutocompleteContext{
		Client:      c,
		Router:      r,
		Command:     cmd,
		Interaction: i,
		Option:      decl,
		Value:       options.Get(focused.Name).Raw(),
		Options:     options,
		ctx:         tctx,
	}

	choices, err := r.runAutocomplete(ctx, respond)
	if err != nil && r.cfg.AutocompleteErrorHandler != nil {
		r.cfg.AutocompleteErrorHandler(ctx, err)
	}

	_ = respond(discord.InteractionResponseTypeAutocompleteResult,
		discord.AutocompleteResult{Choices: autocompleteChoices(decl.Type, choices)})
}

// runAutocomplete gates the handler behind the command's checks and turns
// panics into errors. A failed check yields no choices and no error.
func (r *Router) runAutocomplete(ac *AutocompleteContext, respond events.InteractionResponderFunc) (choices []Choice, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			choices, err = nil, &PanicError{Value: rec, Stack: debug.Stack()}
		}
	}()

	cctx := newContext(r, ac.Client, ac.Command)
	cctx.Source = SourceInteraction
	cctx.Interaction = ac.Interaction
	cctx.responder = respond
	cctx.Options = ac.Options
	cctx.SetContext(ac.ctx)

	_, checks := r.pipeline(ac.Command)
	for _, chk := range checks {
		if chk(cctx) != nil {
			return nil, nil
		}
	}

	return ac.Option.Autocomplete(ac)
}

// autocompleteChoices fits choices to Discord's limits and the option's
// type. It never returns nil, since Discord rejects a null choice list.
func autocompleteChoices(t OptionType, choices []Choice) []discord.AutocompleteChoice {
	out := make([]discord.AutocompleteChoice, 0, min(len(choices), maxEntries))
	for _, c := range choices {
		if len(out) == maxEntries {
			break
		}
		name := c.Name
		if utf8.RuneCountInString(name) > maxChoiceLength {
			name = string([]rune(name)[:maxChoiceLength-1]) + "…"
		}
		if name == "" {
			continue
		}

		switch t {
		case OptionInteger:
			out = append(out, discord.AutocompleteChoiceInt{Name: name, Value: int(toFloat(c.Value))})
		case OptionNumber:
			out = append(out, discord.AutocompleteChoiceFloat{Name: name, Value: toFloat(c.Value)})
		default:
			v := fmt.Sprint(c.Value)
			if v == "" || utf8.RuneCountInString(v) > maxChoiceLength {
				continue
			}
			out = append(out, discord.AutocompleteChoiceString{Name: name, Value: v})
		}
	}
	return out
}
