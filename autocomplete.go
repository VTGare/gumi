package gumi

import (
	"context"
	"fmt"
	"runtime/debug"
	"time"
	"unicode/utf8"

	"github.com/bwmarrin/discordgo"
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
	Session     *discordgo.Session
	Router      *Router
	Command     *Command
	Interaction *discordgo.InteractionCreate
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

func (c *AutocompleteContext) GuildID() string { return c.Interaction.GuildID }

func (c *AutocompleteContext) ChannelID() string { return c.Interaction.ChannelID }

// UserID returns the ID of the user who is typing.
func (c *AutocompleteContext) UserID() string {
	if m := c.Interaction.Member; m != nil && m.User != nil {
		return m.User.ID
	}
	if c.Interaction.User != nil {
		return c.Interaction.User.ID
	}
	return ""
}

func (c *AutocompleteContext) Locale() discordgo.Locale { return c.Interaction.Locale }

// handleAutocomplete answers with the focused option's suggestions. The
// command's checks run first, so users who couldn't run the command get
// no suggestions; middleware and cooldowns don't run.
func (r *Router) handleAutocomplete(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.ApplicationCommandData()

	r.mu.RLock()
	cmd := r.slashIndex[slashKey{commandTypeOf(data.CommandType), data.Name}]
	r.mu.RUnlock()

	opts := data.Options
	for cmd != nil && cmd.IsGroup() {
		if len(opts) == 0 {
			return
		}
		cmd = cmd.Subcommand(opts[0].Name)
		opts = opts[0].Options
	}
	if cmd == nil {
		return
	}

	var focused *discordgo.ApplicationCommandInteractionDataOption
	for _, o := range opts {
		if o != nil && o.Focused {
			focused = o
			break
		}
	}
	if focused == nil {
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

	options := optionsFromInteraction(s, i.GuildID, opts, data.Resolved)
	ctx := &AutocompleteContext{
		Session:     s,
		Router:      r,
		Command:     cmd,
		Interaction: i,
		Option:      decl,
		Value:       fmt.Sprint(focused.Value),
		Options:     options,
		ctx:         tctx,
	}

	choices, err := r.runAutocomplete(ctx)
	if err != nil && r.cfg.AutocompleteErrorHandler != nil {
		r.cfg.AutocompleteErrorHandler(ctx, err)
	}

	_ = respondAutocomplete(s, i.Interaction, autocompleteChoices(choices))
}

// autocompleteResponse is sent instead of discordgo.InteractionResponse,
// whose data omits an empty choice list and adds message fields.
type autocompleteResponse struct {
	Type discordgo.InteractionResponseType `json:"type"`
	Data struct {
		Choices []*discordgo.ApplicationCommandOptionChoice `json:"choices"`
	} `json:"data"`
}

func respondAutocomplete(s *discordgo.Session, i *discordgo.Interaction, choices []*discordgo.ApplicationCommandOptionChoice) error {
	resp := autocompleteResponse{Type: discordgo.InteractionApplicationCommandAutocompleteResult}
	resp.Data.Choices = choices

	endpoint := discordgo.EndpointInteractionResponse(i.ID, i.Token)
	_, err := s.RequestWithBucketID("POST", endpoint, resp, endpoint)
	return err
}

// runAutocomplete gates the handler behind the command's checks and turns
// panics into errors. A failed check yields no choices and no error.
func (r *Router) runAutocomplete(ac *AutocompleteContext) (choices []Choice, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			choices, err = nil, &PanicError{Value: rec, Stack: debug.Stack()}
		}
	}()

	cctx := newContext(r, ac.Session, ac.Command)
	cctx.Source = SourceInteraction
	cctx.Interaction = ac.Interaction
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

// autocompleteChoices fits choices to Discord's limits. It never returns
// nil, since Discord rejects a null choice list.
func autocompleteChoices(choices []Choice) []*discordgo.ApplicationCommandOptionChoice {
	out := make([]*discordgo.ApplicationCommandOptionChoice, 0, min(len(choices), maxEntries))
	for _, c := range choices {
		if len(out) == maxEntries {
			break
		}

		if v, ok := c.Value.(string); ok && (v == "" || utf8.RuneCountInString(v) > maxChoiceLength) {
			continue
		}

		name := c.Name
		if utf8.RuneCountInString(name) > maxChoiceLength {
			name = string([]rune(name)[:maxChoiceLength-1]) + "…"
		}
		if name == "" {
			continue
		}

		out = append(out, &discordgo.ApplicationCommandOptionChoice{Name: name, Value: c.Value})
	}

	return out
}
