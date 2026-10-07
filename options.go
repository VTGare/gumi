package gumi

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
)

type OptionType = discord.ApplicationCommandOptionType

// Subcommand types are deliberately absent: use Command.Subcommands.
const (
	OptionString      OptionType = discord.ApplicationCommandOptionTypeString
	OptionInteger     OptionType = discord.ApplicationCommandOptionTypeInt
	OptionBoolean     OptionType = discord.ApplicationCommandOptionTypeBool
	OptionUser        OptionType = discord.ApplicationCommandOptionTypeUser
	OptionChannel     OptionType = discord.ApplicationCommandOptionTypeChannel
	OptionRole        OptionType = discord.ApplicationCommandOptionTypeRole
	OptionMentionable OptionType = discord.ApplicationCommandOptionTypeMentionable
	OptionNumber      OptionType = discord.ApplicationCommandOptionTypeFloat
	OptionAttachment  OptionType = discord.ApplicationCommandOptionTypeAttachment
)

type Choice struct {
	Name  string
	Value any
}

// Option is one command argument; the same declaration feeds the slash
// definition and the prefix parser.
type Option struct {
	Type        OptionType
	Name        string
	Description string
	Required    bool
	Choices     []Choice
	// ChannelTypes restricts channel options to the given types.
	ChannelTypes []discord.ChannelType
	// MinValue / MaxValue apply to integer and number options.
	MinValue *float64
	MaxValue *float64
	// MinLength / MaxLength apply to string options.
	MinLength *int
	MaxLength *int
	// Consume the rest of the message over prefix (string only). Only
	// attachments and slash-only options may follow it. Ignored by slash
	// commands.
	Rest bool
	// Skip the option over prefix, where it keeps its zero value. Lets a
	// slash command take extra options without breaking the positional
	// prefix syntax. Cannot be required.
	NoPrefix bool
	// Autocomplete suggests values while the user types (string, integer
	// and number options only; not with Choices). Ignored by prefix
	// commands.
	Autocomplete AutocompleteHandler
}

// Declare options fluently, e.g.
//
//	gumi.String("query", "What to search for").Require().Greedy()

func String(name, description string) *Option {
	return &Option{Type: OptionString, Name: name, Description: description}
}

func Integer(name, description string) *Option {
	return &Option{Type: OptionInteger, Name: name, Description: description}
}

func Number(name, description string) *Option {
	return &Option{Type: OptionNumber, Name: name, Description: description}
}

func Boolean(name, description string) *Option {
	return &Option{Type: OptionBoolean, Name: name, Description: description}
}

func User(name, description string) *Option {
	return &Option{Type: OptionUser, Name: name, Description: description}
}

func Channel(name, description string) *Option {
	return &Option{Type: OptionChannel, Name: name, Description: description}
}

func Role(name, description string) *Option {
	return &Option{Type: OptionRole, Name: name, Description: description}
}

func Mentionable(name, description string) *Option {
	return &Option{Type: OptionMentionable, Name: name, Description: description}
}

func Attachment(name, description string) *Option {
	return &Option{Type: OptionAttachment, Name: name, Description: description}
}

func (o *Option) Require() *Option { o.Required = true; return o }

func (o *Option) Greedy() *Option { o.Rest = true; return o }

func (o *Option) SlashOnly() *Option { o.NoPrefix = true; return o }

func (o *Option) WithChoices(choices ...Choice) *Option { o.Choices = choices; return o }

func (o *Option) WithRange(minValue, maxValue float64) *Option {
	o.MinValue, o.MaxValue = &minValue, &maxValue
	return o
}

func (o *Option) WithLength(minLength, maxLength int) *Option {
	o.MinLength, o.MaxLength = &minLength, &maxLength
	return o
}

func (o *Option) WithAutocomplete(h AutocompleteHandler) *Option { o.Autocomplete = h; return o }

func (o *Option) WithChannelTypes(types ...discord.ChannelType) *Option {
	o.ChannelTypes = types
	return o
}

func (o *Option) usage() string {
	name := o.Name
	if o.Rest {
		name += "..."
	}
	if o.Required {
		return "<" + name + ">"
	}
	return "[" + name + "]"
}

func (o *Option) toDiscord() discord.ApplicationCommandOption {
	auto := o.Autocomplete != nil
	switch o.Type {
	case OptionString:
		d := discord.ApplicationCommandOptionString{
			Name: o.Name, Description: o.Description, Required: o.Required, Autocomplete: auto,
			MinLength: o.MinLength, MaxLength: o.MaxLength,
		}
		for _, c := range o.Choices {
			d.Choices = append(d.Choices, discord.ApplicationCommandOptionChoiceString{Name: c.Name, Value: fmt.Sprint(c.Value)})
		}
		return d
	case OptionInteger:
		d := discord.ApplicationCommandOptionInt{
			Name: o.Name, Description: o.Description, Required: o.Required, Autocomplete: auto,
			MinValue: intPtr(o.MinValue), MaxValue: intPtr(o.MaxValue),
		}
		for _, c := range o.Choices {
			d.Choices = append(d.Choices, discord.ApplicationCommandOptionChoiceInt{Name: c.Name, Value: int(toFloat(c.Value))})
		}
		return d
	case OptionNumber:
		d := discord.ApplicationCommandOptionFloat{
			Name: o.Name, Description: o.Description, Required: o.Required, Autocomplete: auto,
			MinValue: o.MinValue, MaxValue: o.MaxValue,
		}
		for _, c := range o.Choices {
			d.Choices = append(d.Choices, discord.ApplicationCommandOptionChoiceFloat{Name: c.Name, Value: toFloat(c.Value)})
		}
		return d
	case OptionBoolean:
		return discord.ApplicationCommandOptionBool{Name: o.Name, Description: o.Description, Required: o.Required}
	case OptionUser:
		return discord.ApplicationCommandOptionUser{Name: o.Name, Description: o.Description, Required: o.Required}
	case OptionChannel:
		return discord.ApplicationCommandOptionChannel{
			Name: o.Name, Description: o.Description, Required: o.Required, ChannelTypes: o.ChannelTypes,
		}
	case OptionRole:
		return discord.ApplicationCommandOptionRole{Name: o.Name, Description: o.Description, Required: o.Required}
	case OptionMentionable:
		return discord.ApplicationCommandOptionMentionable{Name: o.Name, Description: o.Description, Required: o.Required}
	default:
		return discord.ApplicationCommandOptionAttachment{Name: o.Name, Description: o.Description, Required: o.Required}
	}
}

func intPtr(f *float64) *int {
	if f == nil {
		return nil
	}
	n := int(math.Round(*f))
	return &n
}

func validateOptions(opts []*Option) error {
	seenOptional := false
	names := make(map[string]bool)

	for i, o := range opts {
		if o == nil {
			return errors.New("nil option")
		}
		if !validChatName(o.Name) {
			return fmt.Errorf("option %q: names must be 1-%d lowercase characters without spaces", o.Name, maxNameLength)
		}

		if !validDescription(o.Description) {
			return fmt.Errorf("option %q: description must be 1-%d characters", o.Name, maxDescriptionLength)
		}
		if names[o.Name] {
			return fmt.Errorf("option %q: duplicate option name", o.Name)
		}
		names[o.Name] = true

		switch o.Type {
		case OptionString, OptionInteger, OptionBoolean, OptionUser, OptionChannel,
			OptionRole, OptionMentionable, OptionNumber, OptionAttachment:
		default:
			return fmt.Errorf("option %q: unsupported option type %d (use Subcommands for subcommands)", o.Name, o.Type)
		}

		if o.Required && seenOptional {
			return fmt.Errorf("option %q: required options must be declared before optional ones", o.Name)
		}

		if !o.Required {
			seenOptional = true
		}

		if o.NoPrefix && o.Required {
			return fmt.Errorf("option %q: slash-only options cannot be required", o.Name)
		}

		if o.Rest {
			if o.Type != OptionString {
				return fmt.Errorf("option %q: only string options can be greedy", o.Name)
			}

			for _, after := range opts[i+1:] {
				if after != nil && after.Type != OptionAttachment && !after.NoPrefix {
					return fmt.Errorf("option %q: only attachments and slash-only options may follow a greedy option", o.Name)
				}
			}
		}
		if len(o.Choices) > maxEntries {
			return fmt.Errorf("option %q: at most %d choices are allowed", o.Name, maxEntries)
		}

		if o.Autocomplete != nil {
			switch {
			case o.Type != OptionString && o.Type != OptionInteger && o.Type != OptionNumber:
				return fmt.Errorf("option %q: only string, integer and number options can autocomplete", o.Name)
			case len(o.Choices) > 0:
				return fmt.Errorf("option %q: autocomplete cannot be combined with choices", o.Name)
			}
		}
	}
	return nil
}

type Options struct {
	values map[string]*Value
}

func newOptions() *Options { return &Options{values: make(map[string]*Value)} }

func (o *Options) set(name string, v *Value) { o.values[name] = v }

func (o *Options) Has(name string) bool {
	_, ok := o.values[name]
	return ok
}

func (o *Options) Len() int { return len(o.values) }

func (o *Options) Get(name string) *Value { return o.values[name] }

func (o *Options) String(name string) string { return o.Get(name).String() }

func (o *Options) StringOr(name, def string) string {
	if v := o.Get(name); v != nil {
		return v.String()
	}
	return def
}

func (o *Options) Int(name string) int64 { return o.Get(name).Int() }

func (o *Options) IntOr(name string, def int64) int64 {
	if v := o.Get(name); v != nil {
		return v.Int()
	}
	return def
}

func (o *Options) Float(name string) float64 { return o.Get(name).Float() }

func (o *Options) Bool(name string) bool { return o.Get(name).Bool() }

func (o *Options) BoolOr(name string, def bool) bool {
	if v := o.Get(name); v != nil {
		return v.Bool()
	}
	return def
}

func (o *Options) ID(name string) snowflake.ID { return o.Get(name).ID() }

// Entity getters return nil when the option is absent or unresolvable;
// call the Value's method directly to see why.

func (o *Options) User(name string) *discord.User {
	u, _ := o.Get(name).User()
	return u
}

func (o *Options) Member(name string) *discord.Member {
	m, _ := o.Get(name).Member()
	return m
}

func (o *Options) Channel(name string) *discord.ResolvedChannel {
	c, _ := o.Get(name).Channel()
	return c
}

func (o *Options) Role(name string) *discord.Role {
	r, _ := o.Get(name).Role()
	return r
}

func (o *Options) Attachment(name string) *discord.Attachment {
	return o.Get(name).Attachment()
}

type mentionKind uint8

const (
	kindUnknown mentionKind = iota
	kindUser
	kindRole
)

// Prefix invocations look entities up when asked, in the cache and then
// over REST. Slash commands come with them resolved.
type Value struct {
	Type OptionType

	raw  string
	str  string
	num  float64
	b    bool
	id   snowflake.ID
	kind mentionKind

	user       *discord.User
	member     *discord.Member
	channel    *discord.ResolvedChannel
	role       *discord.Role
	attachment *discord.Attachment

	client  *bot.Client
	guildID snowflake.ID
}

// Raw is the value as typed over prefix, or its string form from slash.
func (v *Value) Raw() string {
	if v == nil {
		return ""
	}
	return v.raw
}

// String is the string value, or Raw() for anything else.
func (v *Value) String() string {
	if v == nil {
		return ""
	}
	if v.Type == OptionString {
		return v.str
	}
	return v.raw
}

func (v *Value) Int() int64 {
	if v == nil {
		return 0
	}
	return int64(v.num)
}

func (v *Value) Float() float64 {
	if v == nil {
		return 0
	}
	return v.num
}

func (v *Value) Bool() bool {
	if v == nil {
		return false
	}
	return v.b
}

func (v *Value) ID() snowflake.ID {
	if v == nil {
		return 0
	}
	return v.id
}

func (v *Value) IsRole() bool { return v != nil && (v.role != nil || v.kind == kindRole) }

func (v *Value) IsUser() bool {
	return v != nil && (v.user != nil || v.member != nil || v.kind == kindUser)
}

func (v *Value) User() (*discord.User, error) {
	if v == nil || v.id == 0 {
		return nil, ErrMissingOption
	}

	if v.user != nil {
		return v.user, nil
	}

	if v.member != nil {
		return &v.member.User, nil
	}

	if v.client == nil || v.kind == kindRole {
		return nil, fmt.Errorf("gumi: cannot resolve user %s", v.id)
	}

	u, err := v.client.Rest.GetUser(v.id)
	if err != nil {
		return nil, err
	}

	v.user = u
	return u, nil
}

func (v *Value) Member() (*discord.Member, error) {
	if v == nil || v.id == 0 {
		return nil, ErrMissingOption
	}

	if v.member != nil {
		return v.member, nil
	}

	if v.client == nil || v.guildID == 0 || v.kind == kindRole {
		return nil, fmt.Errorf("gumi: cannot resolve member %s", v.id)
	}

	if v.client.Caches != nil {
		if m, ok := v.client.Caches.Member(v.guildID, v.id); ok {
			v.member = &m
			return &m, nil
		}
	}

	m, err := v.client.Rest.GetMember(v.guildID, v.id)
	if err != nil {
		return nil, err
	}

	v.member = m
	return m, nil
}

func (v *Value) Channel() (*discord.ResolvedChannel, error) {
	if v == nil || v.id == 0 {
		return nil, ErrMissingOption
	}

	if v.channel != nil {
		return v.channel, nil
	}

	if v.client == nil {
		return nil, fmt.Errorf("gumi: cannot resolve channel %s", v.id)
	}

	var ch discord.Channel
	if v.client.Caches != nil {
		if c, ok := v.client.Caches.Channel(v.id); ok {
			ch = c
		}
	}

	if ch == nil {
		c, err := v.client.Rest.GetChannel(v.id)
		if err != nil {
			return nil, err
		}
		ch = c
	}

	v.channel = resolvedChannel(ch)
	return v.channel, nil
}

func resolvedChannel(ch discord.Channel) *discord.ResolvedChannel {
	r := &discord.ResolvedChannel{ID: ch.ID(), Name: ch.Name(), Type: ch.Type()}
	if gc, ok := ch.(discord.GuildChannel); ok && gc.ParentID() != nil {
		r.ParentID = *gc.ParentID()
	}
	return r
}

func (v *Value) Role() (*discord.Role, error) {
	if v == nil || v.id == 0 {
		return nil, ErrMissingOption
	}

	if v.role != nil {
		return v.role, nil
	}

	if v.client == nil || v.guildID == 0 || v.kind == kindUser {
		return nil, fmt.Errorf("gumi: cannot resolve role %s", v.id)
	}

	if v.client.Caches != nil {
		if r, ok := v.client.Caches.Role(v.guildID, v.id); ok {
			v.role = &r
			return &r, nil
		}
	}

	r, err := v.client.Rest.GetRole(v.guildID, v.id)
	if err != nil {
		return nil, err
	}

	v.role = r
	return r, nil
}

func (v *Value) Attachment() *discord.Attachment {
	if v == nil {
		return nil
	}
	return v.attachment
}

func optionsFromSlash(c *bot.Client, guildID snowflake.ID, opts map[string]discord.SlashCommandOption, resolved *discord.ResolvedData) *Options {
	out := newOptions()
	for name, opt := range opts {
		out.set(name, interactionValue(c, guildID, opt.Type, opt.Value, resolved))
	}
	return out
}

// Autocomplete sends what's typed into a number option as a string, so
// numbers parse from strings too.
func interactionValue(c *bot.Client, guildID snowflake.ID, t OptionType, raw json.RawMessage, resolved *discord.ResolvedData) *Value {
	v := &Value{Type: t, client: c, guildID: guildID}

	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		text = string(raw)
	}

	switch t {
	case OptionString:
		v.str, v.raw = text, text
	case OptionInteger, OptionNumber:
		v.num, _ = strconv.ParseFloat(text, 64)
		v.raw = text
	case OptionBoolean:
		v.b, _ = strconv.ParseBool(text)
		v.raw = strconv.FormatBool(v.b)
	default:
		v.id, _ = snowflake.Parse(text)
		v.raw = text
		if resolved != nil {
			resolve(v, resolved)
		}
	}

	return v
}

func resolve(v *Value, resolved *discord.ResolvedData) {
	if u, ok := resolved.Users[v.id]; ok {
		v.user = &u
	}
	if m, ok := resolved.Members[v.id]; ok {
		if v.user != nil {
			m.User = *v.user
		}
		v.member = &m.Member
	}
	if ch, ok := resolved.Channels[v.id]; ok {
		v.channel = &ch
	}
	if r, ok := resolved.Roles[v.id]; ok {
		v.role = &r
	}
	if a, ok := resolved.Attachments[v.id]; ok {
		v.attachment = &a
	}

	switch {
	case v.role != nil:
		v.kind = kindRole
	case v.user != nil || v.member != nil:
		v.kind = kindUser
	}
}

func toFloat(x any) float64 {
	switch n := x.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case int32:
		return float64(n)
	case string:
		f, _ := strconv.ParseFloat(n, 64)
		return f
	}
	return 0
}
