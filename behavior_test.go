package gumi

import (
	"strings"
	"time"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"

	"github.com/VTGare/gumi/v2/gumitest"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
)

var _ = ginkgo.Describe("Parsing prefix values", func() {
	parse := func(o *Option, text string) (*Value, error) {
		return parseValue(testCtx(), o, text)
	}

	ginkgo.DescribeTable("maps choices by name or value, case-insensitively",
		func(o *Option, text string, check func(*Value)) {
			v, err := parse(o, text)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			check(v)
		},
		ginkgo.Entry("string by name", String("s", "d").WithChoices(Choice{"Fast", "fast"}), "FAST",
			func(v *Value) { gomega.Expect(v.String()).To(gomega.Equal("fast")) }),
		ginkgo.Entry("integer by name", Integer("i", "d").WithChoices(Choice{"Ten", 10}), "ten",
			func(v *Value) { gomega.Expect(v.Int()).To(gomega.Equal(int64(10))) }),
		ginkgo.Entry("integer by value", Integer("i", "d").WithChoices(Choice{"Ten", 10}), "10",
			func(v *Value) { gomega.Expect(v.Int()).To(gomega.Equal(int64(10))) }),
		ginkgo.Entry("number by name", Number("n", "d").WithChoices(Choice{"Half", 0.5}), "half",
			func(v *Value) { gomega.Expect(v.Float()).To(gomega.Equal(0.5)) }),
	)

	ginkgo.DescribeTable("rejects invalid values",
		func(o *Option, text, msg string) {
			_, err := parse(o, text)
			gomega.Expect(err).To(gomega.MatchError(gomega.ContainSubstring(msg)))
		},
		ginkgo.Entry("unknown choice", Integer("i", "d").WithChoices(Choice{"Ten", 10}, Choice{"Five", 5}), "3", "expected one of Ten, Five"),
		ginkgo.Entry("not an integer", Integer("i", "d"), "1.5", "expected a whole number"),
		ginkgo.Entry("not a number", Number("n", "d"), "x", "expected a number"),
		ginkgo.Entry("number out of range", Number("n", "d").WithRange(0, 1), "2", "at most 1"),
		ginkgo.Entry("string too short", String("s", "d").WithLength(3, 5), "ab", "at least 3 characters"),
		ginkgo.Entry("string too long", String("s", "d").WithLength(3, 5), "abcdef", "at most 5 characters"),
		ginkgo.Entry("not a boolean", Boolean("b", "d"), "maybe", "expected yes/no"),
		ginkgo.Entry("not a user", User("u", "d"), "bob", "expected a user mention or ID"),
		ginkgo.Entry("not a role", Role("r", "d"), "<#123456789012345678>", "expected a role mention or ID"),
		ginkgo.Entry("not a mentionable", Mentionable("m", "d"), "bob", "expected a user or role mention"),
	)

	ginkgo.DescribeTable("resolves mentionables to users or roles",
		func(text string, id snowflake.ID, user, role bool) {
			v, err := parse(Mentionable("m", "d"), text)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(v.ID()).To(gomega.Equal(id))
			gomega.Expect(v.IsUser()).To(gomega.Equal(user))
			gomega.Expect(v.IsRole()).To(gomega.Equal(role))
		},
		ginkgo.Entry("user mention", "<@123456789012345678>", snowflake.ID(123456789012345678), true, false),
		ginkgo.Entry("nick mention", "<@!123456789012345678>", snowflake.ID(123456789012345678), true, false),
		ginkgo.Entry("role mention", "<@&123456789012345678>", snowflake.ID(123456789012345678), false, true),
		ginkgo.Entry("bare ID", "123456789012345678", snowflake.ID(123456789012345678), false, false),
	)

	ginkgo.It("parses booleans in several spellings", func() {
		for _, text := range []string{"yes", "ON", "enabled", "1"} {
			v, err := parse(Boolean("b", "d"), text)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(v.Bool()).To(gomega.BeTrue())
		}
	})
})

var _ = ginkgo.Describe("Reading options", func() {
	ginkgo.It("returns zero values and defaults for absent options", func() {
		opts := newOptions()

		gomega.Expect(opts.String("x")).To(gomega.BeEmpty())
		gomega.Expect(opts.StringOr("x", "def")).To(gomega.Equal("def"))
		gomega.Expect(opts.IntOr("x", 7)).To(gomega.Equal(int64(7)))
		gomega.Expect(opts.User("x")).To(gomega.BeNil())
		gomega.Expect(opts.Member("x")).To(gomega.BeNil())
		gomega.Expect(opts.Channel("x")).To(gomega.BeNil())
		gomega.Expect(opts.Role("x")).To(gomega.BeNil())
		gomega.Expect(opts.Attachment("x")).To(gomega.BeNil())
	})

	ginkgo.It("maps interaction options with resolved entities", func() {
		i := gumitest.Command(1, "x", gumitest.User("who", 7), gumitest.Int("n", 3)).
			WithResolved("users", map[string]any{"7": map[string]any{"id": "7", "username": "vt"}}).
			Parse().(discord.ApplicationCommandInteraction)
		data := i.SlashCommandInteractionData()
		opts := optionsFromSlash(nil, 0, data.Options, &data.Resolved)

		gomega.Expect(opts.User("who").Username).To(gomega.Equal("vt"))
		gomega.Expect(opts.Member("who")).To(gomega.BeNil())
		gomega.Expect(opts.Int("n")).To(gomega.Equal(int64(3)))
		gomega.Expect(opts.Get("n").Raw()).To(gomega.Equal("3"))
	})
})

var _ = ginkgo.Describe("Matching prefixes", func() {
	client := func() *bot.Client {
		c, _ := gumitest.NewClient()
		return c
	}
	msg := func(content string) discord.Message { return discord.Message{Content: content} }

	ginkgo.DescribeTable("picks the longest matching prefix",
		func(prefixes []string, content, wantPrefix, wantRest string, wantOK bool) {
			r := New(Config{Prefixes: prefixes})
			p, rest, ok := r.matchPrefix(client(), msg(content))

			gomega.Expect(ok).To(gomega.Equal(wantOK))
			gomega.Expect(p).To(gomega.Equal(wantPrefix))
			gomega.Expect(rest).To(gomega.Equal(wantRest))
		},
		ginkgo.Entry("longest wins", []string{"bt", "bt!"}, "bt!help", "bt!", "help", true),
		ginkgo.Entry("case-insensitive", []string{"bt!"}, "BT!help", "bt!", "help", true),
		ginkgo.Entry("mention", []string{"bt!"}, "<@1001> help", "<@1001>", " help", true),
		ginkgo.Entry("nick mention", []string{"bt!"}, "<@!1001>help", "<@!1001>", "help", true),
		ginkgo.Entry("no match", []string{"bt!"}, "hello", "", "", false),
		ginkgo.Entry("empty prefixes skipped", []string{""}, "hello", "", "", false),
	)

	ginkgo.It("ignores mentions when disabled", func() {
		r := New(Config{Prefixes: []string{"bt!"}, DisableMentionPrefix: true})
		_, _, ok := r.matchPrefix(client(), msg("<@1001> help"))
		gomega.Expect(ok).To(gomega.BeFalse())
	})
})

var _ = ginkgo.Describe("Permission checks", func() {
	interactionCtx := func(inGuild bool, member, app discord.Permissions) *Context {
		i := gumitest.Command(1, "x").WithPermissions(member).WithAppPermissions(app)
		if !inGuild {
			i.InDM(9)
		}
		return &Context{Interaction: i.Parse()}
	}

	ginkgo.DescribeTable("HasPermissions",
		func(inGuild bool, perms discord.Permissions, pass bool) {
			err := HasPermissions(discord.PermissionManageMessages)(interactionCtx(inGuild, perms, 0))
			if pass {
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
			} else {
				gomega.Expect(err).To(gomega.BeAssignableToTypeOf(&CheckError{}))
				gomega.Expect(err.(*CheckError).Check).To(gomega.Equal("permissions"))
			}
		},
		ginkgo.Entry("has it", true, discord.PermissionManageMessages, true),
		ginkgo.Entry("admin", true, discord.PermissionAdministrator, true),
		ginkgo.Entry("lacks it", true, discord.PermissionSendMessages, false),
		ginkgo.Entry("outside guilds", false, discord.Permissions(0), true),
	)

	ginkgo.DescribeTable("BotHasPermissions",
		func(perms discord.Permissions, pass bool) {
			err := BotHasPermissions(discord.PermissionEmbedLinks)(interactionCtx(true, 0, perms))
			if pass {
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
			} else {
				gomega.Expect(err.(*CheckError).Check).To(gomega.Equal("bot_permissions"))
			}
		},
		ginkgo.Entry("has it", discord.PermissionEmbedLinks, true),
		ginkgo.Entry("lacks it", discord.Permissions(0), false),
	)

	ginkgo.It("checks NSFW on a thread's parent channel", func() {
		c, _ := gumitest.NewClient()
		guildID := snowflake.ID(10)
		c.Caches.AddChannel(gumitest.GuildChannel(map[string]any{"id": "20", "guild_id": "10", "type": discord.ChannelTypeGuildText, "nsfw": true}))
		c.Caches.AddChannel(gumitest.GuildChannel(map[string]any{
			"id": "30", "guild_id": "10", "parent_id": "20", "type": discord.ChannelTypeGuildPublicThread,
			"thread_metadata": map[string]any{"archive_timestamp": gumitest.Timestamp},
		}))

		ctx := &Context{Client: c, Message: &discord.Message{GuildID: &guildID, ChannelID: 30}}
		gomega.Expect(NSFW(ctx)).To(gomega.Succeed())

		c.Caches.AddChannel(gumitest.GuildChannel(map[string]any{"id": "20", "guild_id": "10", "type": discord.ChannelTypeGuildText}))
		gomega.Expect(NSFW(ctx)).To(gomega.MatchError(gomega.ContainSubstring("nsfw")))
	})
})

var _ = ginkgo.Describe("Cooldowns", func() {
	ginkgo.It("allows Uses per window and reports the wait", func() {
		c := NewCooldown(CooldownUser, 2, time.Minute)

		_, ok := c.Take("u")
		gomega.Expect(ok).To(gomega.BeTrue())
		_, ok = c.Take("u")
		gomega.Expect(ok).To(gomega.BeTrue())

		wait, ok := c.Take("u")
		gomega.Expect(ok).To(gomega.BeFalse())
		gomega.Expect(wait).To(gomega.BeNumerically(">", 50*time.Second))

		_, ok = c.Take("other")
		gomega.Expect(ok).To(gomega.BeTrue())
	})

	ginkgo.It("treats zero uses as one and describes itself", func() {
		c := &Cooldown{Per: 5 * time.Second}

		_, ok := c.Take("u")
		gomega.Expect(ok).To(gomega.BeTrue())
		_, ok = c.Take("u")
		gomega.Expect(ok).To(gomega.BeFalse())
		gomega.Expect(c.String()).To(gomega.Equal("1 use per 5s (per user)"))
		gomega.Expect(NewCooldown(CooldownGuild, 3, time.Second).String()).To(gomega.Equal("3 uses per 1s (per guild)"))
	})

	ginkgo.It("keys buckets by scope", func() {
		guildID := snowflake.ID(1)
		ctx := &Context{Message: &discord.Message{GuildID: &guildID, ChannelID: 2, Author: discord.User{ID: 3}}}
		dm := &Context{Message: &discord.Message{ChannelID: 4, Author: discord.User{ID: 3}}}

		gomega.Expect(NewCooldown(CooldownUser, 1, 0).Key(ctx)).To(gomega.Equal("3"))
		gomega.Expect(NewCooldown(CooldownChannel, 1, 0).Key(ctx)).To(gomega.Equal("2"))
		gomega.Expect(NewCooldown(CooldownGuild, 1, 0).Key(ctx)).To(gomega.Equal("1"))
		gomega.Expect(NewCooldown(CooldownGuild, 1, 0).Key(dm)).To(gomega.Equal("4"))
		gomega.Expect(NewCooldown(CooldownGlobal, 1, 0).Key(ctx)).To(gomega.BeEmpty())
	})
})

var _ = ginkgo.Describe("Response edits", func() {
	ginkgo.It("clears embeds and components that are not set", func() {
		r := Text("hi")

		u := r.messageUpdate()
		gomega.Expect(*u.Content).To(gomega.Equal("hi"))
		gomega.Expect(*u.Embeds).To(gomega.BeEmpty())
		gomega.Expect(*u.Embeds).NotTo(gomega.BeNil())
		gomega.Expect(*u.Components).NotTo(gomega.BeNil())
	})

	ginkgo.It("strips the ephemeral flag over prefix", func() {
		send := Text("hi").Private().channelMessage(nil, nil)
		gomega.Expect(send.Flags.Has(discord.MessageFlagEphemeral)).To(gomega.BeFalse())
	})
})

var _ = ginkgo.Describe("Validating more commands", func() {
	ginkgo.DescribeTable("rejects malformed commands",
		func(cmd *Command, msg string) {
			gomega.Expect(cmd.validate(0)).To(gomega.MatchError(gomega.ContainSubstring(msg)))
		},
		ginkgo.Entry("context menu with options",
			&Command{Name: "Find", Type: MessageContext, Handler: testOK, Options: []*Option{String("a", "d")}},
			"cannot have options"),
		ginkgo.Entry("context menu without handler",
			&Command{Name: "Find", Type: UserContext}, "no handler"),
		ginkgo.Entry("context menu name too long",
			&Command{Name: strings.Repeat("x", 33), Type: UserContext, Handler: testOK}, "1-32 characters"),
		ginkgo.Entry("context menu as subcommand",
			&Command{Name: "g", Description: "d", Subcommands: []*Command{{Name: "Find", Type: MessageContext, Handler: testOK}}},
			"must be a chat input command"),
		ginkgo.Entry("group with options",
			&Command{Name: "g", Description: "d", Options: []*Option{String("a", "d")}, Subcommands: []*Command{{Name: "a", Description: "d", Handler: testOK}}},
			"group commands cannot have options"),
		ginkgo.Entry("duplicate subcommand alias",
			&Command{Name: "g", Description: "d", Subcommands: []*Command{
				{Name: "a", Description: "d", Handler: testOK},
				{Name: "b", Description: "d", Aliases: []string{"A"}, Handler: testOK},
			}},
			"duplicate subcommand name or alias"),
		ginkgo.Entry("alias with whitespace",
			&Command{Name: "a", Description: "d", Aliases: []string{"b c"}, Handler: testOK}, "contain no whitespace"),
		ginkgo.Entry("too many choices",
			&Command{Name: "a", Description: "d", Handler: testOK, Options: []*Option{String("s", "d").WithChoices(make([]Choice, 26)...)}},
			"at most 25 choices"),
		ginkgo.Entry("greedy non-string",
			&Command{Name: "a", Description: "d", Handler: testOK, Options: []*Option{Integer("i", "d").Greedy()}},
			"only string options can be greedy"),
	)
})

var _ = ginkgo.Describe("Subcommand errors", func() {
	ginkgo.It("match the subcommand sentinels", func() {
		group := &Command{Name: "set"}

		gomega.Expect(&SubcommandError{Command: group}).To(gomega.MatchError(ErrMissingSubcommand))
		gomega.Expect(&SubcommandError{Command: group, Given: "x"}).To(gomega.MatchError(ErrUnknownSubcommand))
	})
})
