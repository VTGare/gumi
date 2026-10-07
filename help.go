package gumi

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
)

// HelpConfig customises the built-in help command.
type HelpConfig struct {
	// Name of the command (default "help").
	Name string
	// Description (default "Shows the list of commands or details about one").
	Description string
	// Category the help command itself is listed under (default "General").
	Category string
	// Aliases for prefix invocation.
	Aliases []string
	// Title of the overview embed (default "Commands").
	Title string
	// Color of the embeds.
	Color int
	// Footer appended to the overview embed.
	Footer string
	// ShowHidden lists hidden commands too.
	ShowHidden bool
	// Public makes help replies visible to everyone (default: ephemeral).
	Public bool
	// Uncategorized is the category name for commands without one (default "Other").
	Uncategorized string
	// CategoryOrder lists categories first, in this order; the rest follow
	// alphabetically.
	CategoryOrder []string
}

// Discord limits that shape the help menus.
const (
	maxSelectOptions = 25
	maxSelectText    = 100
	maxEmbedFields   = 25
	maxDescription   = 4096
	maxShownChoices  = 6
)

// HelpCommand documents every command on the router it runs in:
//
//	/help            – overview with a category menu
//	/help set prefix – details, usage, options, cooldown
//
// The menus are stateless: each custom ID carries the invoker, the syntax
// mode and the target, so they keep working across restarts.
func HelpCommand(cfg HelpConfig) *Command {
	cfg.Name = cmp.Or(cfg.Name, "help")
	cfg.Description = cmp.Or(cfg.Description, "Shows the list of commands or details about one")
	cfg.Category = cmp.Or(cfg.Category, "General")
	cfg.Title = cmp.Or(cfg.Title, "Commands")
	cfg.Uncategorized = cmp.Or(cfg.Uncategorized, "Other")

	help := &Command{
		Name:        cfg.Name,
		Description: cfg.Description,
		Category:    cfg.Category,
		Aliases:     cfg.Aliases,
		Ephemeral:   !cfg.Public,
		Options: []*Option{
			String("command", "Command to show details for, e.g. \"set prefix\"").Greedy(),
		},
	}

	help.Handler = func(ctx *Context) error {
		v := newHelpView(ctx, cfg, help)

		query := strings.TrimSpace(ctx.Options.String("command"))
		if query == "" {
			return ctx.Reply(v.home())
		}

		cmd := v.lookup(query, ctx.DisplayPrefix())
		if cmd == nil {
			return Errorf("Command `%s` not found.", query)
		}

		return ctx.Reply(v.detail(cmd))
	}

	help.Components = func(ctx *ComponentContext) error {
		resp, private := helpComponent(ctx, cfg, help)
		if private {
			return ctx.Reply(resp)
		}

		return ctx.Update(resp)
	}

	return help
}

// helpView renders help for one invoker, in the syntax they invoked it with.
type helpView struct {
	r    *Router
	c    *bot.Client
	cfg  HelpConfig
	help *Command

	guildID, channelID snowflake.ID
	userID             snowflake.ID
	// prefix is true when help was run as a prefix command, so prefix syntax
	// is shown first.
	prefix bool
	// usedPrefix is the prefix help was invoked with, if any.
	usedPrefix string
}

func newHelpView(ctx *Context, cfg HelpConfig, help *Command) *helpView {
	return &helpView{
		r:          ctx.Router,
		c:          ctx.Client,
		cfg:        cfg,
		help:       help,
		guildID:    ctx.GuildID(),
		channelID:  ctx.ChannelID(),
		userID:     ctx.AuthorID(),
		prefix:     ctx.IsMessage(),
		usedPrefix: ctx.Prefix,
	}
}

// helpComponent answers a click on a help menu: the next view for the
// invoker, or a private notice (private true) for anyone else.
func helpComponent(ctx *ComponentContext, cfg HelpConfig, help *Command) (resp *Response, private bool) {
	ownerID, mode, action := ctx.Arg(0), ctx.Arg(1), ctx.Arg(2)

	// The target may itself contain ':', so it is everything after the action.
	arg := ""
	if len(ctx.Args) > 3 {
		arg = strings.Join(ctx.Args[3:], ":")
	}
	if values := ctx.Values(); len(values) > 0 {
		arg = values[0]
	}

	if ctx.UserID().String() != ownerID {
		return Textf("This menu belongs to someone else. Run `/%s` for your own.", cfg.Name).Private(), true
	}

	v := &helpView{
		r: ctx.Router, c: ctx.Client, cfg: cfg, help: help,
		guildID: ctx.GuildID(), channelID: ctx.ChannelID(),
		userID: ctx.UserID(), prefix: mode == "p",
	}

	switch action {
	case "cat":
		resp = v.category(arg)
	case "cmd":
		if cmd := v.decodeCommand(arg); cmd != nil {
			resp = v.detail(cmd)
		}
	}
	if resp == nil {
		resp = v.home()
	}

	return resp, false
}

// prefixLead is the guild's first configured prefix, for showing prefix
// syntax in help. It falls back to the used prefix, the mention prefix,
// then "/".
func (v *helpView) prefixLead() string {
	if p := v.r.Prefixes(v.c, v.guildID, v.channelID); len(p) > 0 {
		return p[0]
	}

	return cmp.Or(v.usedPrefix, v.r.mentionPrefix(v.c), "/")
}

// syntaxLead is the lead for c in the invoker's mode.
func (v *helpView) syntaxLead(c *Command) string {
	if !v.prefix && slashEnabled(v.r, c) {
		return "/"
	}

	return v.prefixLead()
}

// helpUsage lists the command's syntax, invoking form first.
func (v *helpView) helpUsage(cmd *Command) []string {
	var lines []string

	addSlash := func() {
		if slashEnabled(v.r, cmd) {
			lines = append(lines, cmd.Usage("/"))
		}
	}

	addPrefix := func() {
		if prefixEnabled(v.r, cmd) {
			if p := v.prefixLead(); p != "" {
				lines = append(lines, cmd.Usage(p))
			}
		}
	}

	if v.prefix {
		addPrefix()
		addSlash()
	} else {
		addSlash()
		addPrefix()
	}

	return lines
}

// lookup resolves a /help query: a command path, or a context menu name.
func (v *helpView) lookup(query, usedPrefix string) *Command {
	fields := strings.Fields(query)
	fields[0] = strings.TrimPrefix(fields[0], "/")
	if usedPrefix != "" {
		fields[0] = strings.TrimPrefix(fields[0], usedPrefix)
	}

	cmd := v.r.Lookup(fields...)
	if cmd == nil {
		for _, c := range v.r.Commands() {
			if c.Type != ChatInput && strings.EqualFold(c.Name, query) {
				cmd = c
				break
			}
		}
	}

	if cmd == nil || !v.visible(cmd) {
		return nil
	}

	return cmd
}

func (v *helpView) visible(c *Command) bool {
	if c.Hidden && !v.cfg.ShowHidden {
		return false
	}

	return slashEnabled(v.r, c) || prefixEnabled(v.r, c)
}

func (v *helpView) categoryOf(c *Command) string {
	return cmp.Or(c.Root().Category, v.cfg.Uncategorized)
}

type helpCategory struct {
	name string
	cmds []*Command
}

// categories groups visible root commands: CategoryOrder first, the rest
// alphabetically. Chat commands come first in each, then context menus.
func (v *helpView) categories() []helpCategory {
	byName := make(map[string][]*Command)
	for _, c := range v.r.Commands() {
		if !v.visible(c) {
			continue
		}

		cat := v.categoryOf(c)
		byName[cat] = append(byName[cat], c)
	}

	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}

	rank := func(name string) int {
		if i := slices.Index(v.cfg.CategoryOrder, name); i >= 0 {
			return i
		}
		return len(v.cfg.CategoryOrder)
	}

	slices.SortFunc(names, func(a, b string) int {
		return cmp.Or(cmp.Compare(rank(a), rank(b)), strings.Compare(a, b))
	})

	out := make([]helpCategory, 0, len(names))
	for _, name := range names {
		cmds := byName[name]
		slices.SortFunc(cmds, func(a, b *Command) int {
			return cmp.Or(cmp.Compare(a.Type, b.Type), strings.Compare(a.Name, b.Name))
		})
		out = append(out, helpCategory{name: name, cmds: cmds})
	}

	return out
}

func (v *helpView) home() *Response {
	cats := v.categories()

	desc := fmt.Sprintf("Pick a category below, or run `%s%s <command>` for details.", v.syntaxLead(v.help), v.help.Name)
	switch {
	case !v.prefix && prefixEnabled(v.r, v.help) && v.prefixLead() != "/":
		desc += fmt.Sprintf(" Commands also work with `%s`.", v.prefixLead())
	case v.prefix && slashEnabled(v.r, v.help):
		desc += " Commands also work as slash commands."
	}

	embed := discord.Embed{Title: v.cfg.Title, Color: v.cfg.Color, Description: desc}
	if v.cfg.Footer != "" {
		embed.Footer = &discord.EmbedFooter{Text: v.cfg.Footer}
	}

	inline := true
	options := make([]discord.StringSelectMenuOption, 0, len(cats))
	for _, cat := range cats {
		if len(embed.Fields) < maxEmbedFields {
			embed.Fields = append(embed.Fields, discord.EmbedField{
				Name: cat.name, Value: countNoun(len(cat.cmds), "command"), Inline: &inline,
			})
		}

		names := make([]string, 0, len(cat.cmds))
		for _, c := range cat.cmds {
			names = append(names, c.Name)
		}

		if len(options) < maxSelectOptions {
			options = append(options, discord.StringSelectMenuOption{
				Label:       truncate(cat.name, maxSelectText),
				Value:       truncate(cat.name, maxSelectText),
				Description: truncate(strings.Join(names, ", "), maxSelectText),
			})
		}
	}

	return helpResponse(embed, v.selectRow("cat", "Browse a category", options))
}

func (v *helpView) category(name string) *Response {
	cats := v.categories()
	idx := slices.IndexFunc(cats, func(c helpCategory) bool { return c.name == name })
	if idx < 0 {
		return nil
	}
	cat := cats[idx]

	lines := make([]string, 0, len(cat.cmds))
	options := make([]discord.StringSelectMenuOption, 0, len(cat.cmds))
	for _, c := range cat.cmds {
		if c.Type == ChatInput {
			lines = append(lines, v.commandRef(c)+" — "+c.Description)
		} else {
			lines = append(lines, fmt.Sprintf("**%s** — %s _(%s)_", c.Name, c.Description, menuHint(c)))
		}

		if len(options) < maxSelectOptions {
			options = append(options, discord.StringSelectMenuOption{
				Label:       truncate(helpLabel(v, c), maxSelectText),
				Value:       encodeCommand(c),
				Description: truncate(c.Description, maxSelectText),
			})
		}
	}

	embed := discord.Embed{
		Author:      &discord.EmbedAuthor{Name: v.cfg.Title},
		Title:       cat.name,
		Color:       v.cfg.Color,
		Description: truncate(strings.Join(lines, "\n"), maxDescription),
	}

	return helpResponse(
		embed,
		v.selectRow("cmd", "Pick a command for details", options),
		buttonRow(v.button("All categories", "home", "")),
	)
}

func (v *helpView) detail(cmd *Command) *Response {
	lead := v.syntaxLead(cmd)

	var desc strings.Builder
	desc.WriteString(cmd.Description)
	if lines := v.helpUsage(cmd); len(lines) > 0 {
		desc.WriteString("\n```\n")
		desc.WriteString(lines[0])
		desc.WriteString("\n```")
		if also := v.alsoLine(cmd); also != "" {
			desc.WriteString("\n-# Also ")
			desc.WriteString(also)
		}
	}

	embed := discord.Embed{
		Author:      &discord.EmbedAuthor{Name: v.categoryOf(cmd)},
		Title:       helpLabel(v, cmd),
		Description: desc.String(),
		Color:       v.cfg.Color,
	}

	if cmd.IsGroup() {
		lines := make([]string, 0, len(cmd.Subcommands))
		for _, sub := range cmd.Subcommands {
			if sub.Hidden && !v.cfg.ShowHidden {
				continue
			}

			lines = append(lines, fmt.Sprintf("`%s` — %s", sub.Name, sub.Description))
		}

		addLinesField(&embed, "Subcommands", lines)
	}

	if len(cmd.Options) > 0 {
		lines := make([]string, 0, len(cmd.Options))
		for _, o := range cmd.Options {
			badge := "optional"
			if o.Required {
				badge = "required"
			}
			if o.Rest {
				badge += ", takes the rest"
			}

			line := fmt.Sprintf("`%s` _%s_ — %s", o.Name, badge, o.Description)
			if len(o.Choices) > 0 {
				line += "\n-# " + choiceSummary(o.Choices)
			}

			lines = append(lines, line)
		}

		addLinesField(&embed, "Options", lines)
	}

	if len(cmd.Examples) > 0 {
		examples := make([]string, 0, len(cmd.Examples))
		for _, ex := range cmd.Examples {
			examples = append(examples, v.example(cmd, lead, ex))
		}

		name := "Example"
		if len(examples) > 1 {
			name = "Examples"
		}

		addLinesField(&embed, name, examples)
	}

	for _, c := range cmd.chain() {
		if c.Cooldown != nil {
			embed.Footer = &discord.EmbedFooter{Text: "Cooldown: " + c.Cooldown.String()}
			break
		}
	}

	cat := v.categoryOf(cmd)
	buttons := []discord.InteractiveComponent{
		v.button("Back to "+cat, "cat", cat),
		v.button("All categories", "home", ""),
	}

	return helpResponse(embed, buttonRow(buttons...))
}

// alsoLine lists the other ways to invoke cmd, besides the leading usage.
func (v *helpView) alsoLine(cmd *Command) string {
	if cmd.Type != ChatInput {
		lines := v.helpUsage(cmd)
		if len(lines) < 2 {
			return ""
		}
		return lines[1]
	}

	var forms []string
	if v.prefix && slashEnabled(v.r, cmd) {
		forms = append(forms, "/"+cmd.QualifiedName())
	}

	if prefixEnabled(v.r, cmd) {
		lead := v.prefixLead()
		parent := strings.Join(cmd.Path()[:len(cmd.Path())-1], " ")
		if parent != "" {
			parent += " "
		}

		if !v.prefix {
			forms = append(forms, lead+cmd.QualifiedName())
		}
		for _, a := range cmd.Aliases {
			forms = append(forms, lead+parent+a)
		}
	}

	if len(forms) == 0 {
		return ""
	}

	return "`" + strings.Join(forms, "`, `") + "`"
}

// example renders one example, as a clickable mention plus arguments when
// the command has a synced ID.
func (v *helpView) example(cmd *Command, lead, ex string) string {
	if !v.prefix {
		if mention := v.r.Mention(cmd); mention != "" {
			if args, ok := strings.CutPrefix(ex, cmd.QualifiedName()); ok {
				if args = strings.TrimSpace(args); args != "" {
					return mention + " `" + args + "`"
				}
				return mention
			}
		}
	}

	return "`" + lead + ex + "`"
}

// commandRef is a chat command as a clickable mention when possible.
func (v *helpView) commandRef(c *Command) string {
	if !v.prefix {
		if mention := v.r.Mention(c); mention != "" {
			return mention
		}
	}

	return "`" + v.syntaxLead(c) + c.QualifiedName() + "`"
}

func helpResponse(embed discord.Embed, rows ...discord.LayoutComponent) *Response {
	components := make([]discord.LayoutComponent, 0, len(rows))
	for _, row := range rows {
		if row != nil {
			components = append(components, row)
		}
	}

	return &Response{Embeds: []discord.Embed{embed}, Components: components}
}

// customID encodes a menu action; "" if it would exceed Discord's limit.
func (v *helpView) customID(action, arg string) string {
	mode := "s"
	if v.prefix {
		mode = "p"
	}

	if arg == "" {
		return ComponentID(v.help, v.userID.String(), mode, action)
	}

	return ComponentID(v.help, v.userID.String(), mode, action, arg)
}

func (v *helpView) selectRow(action, placeholder string, options []discord.StringSelectMenuOption) discord.LayoutComponent {
	id := v.customID(action, "")
	if id == "" || len(options) == 0 {
		return nil
	}

	return discord.NewActionRow(discord.StringSelectMenuComponent{
		CustomID:    id,
		Placeholder: placeholder,
		Options:     options,
	})
}

func (v *helpView) button(label, action, arg string) discord.InteractiveComponent {
	id := v.customID(action, arg)
	if id == "" {
		return nil
	}

	return discord.NewSecondaryButton(truncate(label, 80), id)
}

func buttonRow(buttons ...discord.InteractiveComponent) discord.LayoutComponent {
	kept := slices.DeleteFunc(buttons, func(b discord.InteractiveComponent) bool { return b == nil })
	if len(kept) == 0 {
		return nil
	}

	return discord.NewActionRow(kept...)
}

// encodeCommand is a command's select value: its type and qualified name,
// since context menu names may contain spaces.
func encodeCommand(c *Command) string {
	return truncate(strconv.Itoa(int(c.Type))+":"+c.QualifiedName(), maxSelectText)
}

func (v *helpView) decodeCommand(value string) *Command {
	t, name, ok := strings.Cut(value, ":")
	if !ok {
		return nil
	}

	typ, err := strconv.Atoi(t)
	if err != nil {
		return nil
	}

	var cmd *Command
	if CommandType(typ) == ChatInput {
		cmd = v.r.Lookup(strings.Fields(name)...)
	} else {
		for _, c := range v.r.Commands() {
			if int(c.Type) == typ && c.Name == name {
				cmd = c
				break
			}
		}
	}

	if cmd == nil || int(cmd.Type) != typ || !v.visible(cmd) {
		return nil
	}

	return cmd
}

func slashEnabled(r *Router, c *Command) bool {
	return !r.cfg.DisableSlashCommands && !c.Root().DisableSlash
}

func prefixEnabled(r *Router, c *Command) bool {
	if r.cfg.DisablePrefixCommands {
		return false
	}
	for _, cur := range c.chain() {
		if cur.DisablePrefix {
			return false
		}
	}
	return true
}

func helpLabel(v *helpView, c *Command) string {
	switch c.Type {
	case MessageContext:
		return c.Name + " (message menu)"
	case UserContext:
		return c.Name + " (user menu)"
	}
	return v.syntaxLead(c) + c.QualifiedName()
}

func menuHint(c *Command) string {
	if c.Type == UserContext {
		return "right-click a user, Apps"
	}
	return "right-click a message, Apps"
}

func choiceSummary(choices []Choice) string {
	names := make([]string, 0, maxShownChoices)
	for _, c := range choices[:min(len(choices), maxShownChoices)] {
		names = append(names, c.Name)
	}

	s := strings.Join(names, " · ")
	if extra := len(choices) - maxShownChoices; extra > 0 {
		s += fmt.Sprintf(" · +%d more", extra)
	}

	return s
}

func countNoun(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

func truncate(s string, limit int) string {
	if len([]rune(s)) <= limit {
		return s
	}
	return string([]rune(s)[:limit-1]) + "…"
}

func addLinesField(e *discord.Embed, name string, lines []string) {
	if len(lines) == 0 {
		return
	}

	var b strings.Builder
	first := true
	flush := func() {
		if b.Len() == 0 {
			return
		}
		n := name
		if !first {
			n = "\u200b"
		}
		e.Fields = append(e.Fields, discord.EmbedField{Name: n, Value: b.String()})
		b.Reset()
		first = false
	}
	for _, l := range lines {
		if b.Len()+len(l)+1 > 1024 {
			flush()
		}

		if b.Len() > 0 {
			b.WriteByte('\n')
		}

		b.WriteString(l)
	}

	flush()
}
