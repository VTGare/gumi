package gumi

import (
	"net/http"
	"strings"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/snowflake/v2"

	gt "github.com/VTGare/gumi/v2/gumitest"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
)

var _ = ginkgo.Describe("Running interactions", func() {
	var (
		r   *Router
		c   *bot.Client
		rec *gt.Recorder
	)

	ginkgo.BeforeEach(func() {
		c, rec = gt.NewClient()
		r = New(Config{})
	})

	run := func(i *gt.Interaction) { r.HandleInteraction(i.Event(c)) }

	ginkgo.It("replies with the options and invoker it was given", func() {
		r.MustRegister(&Command{Name: "relay", Description: "d", Subcommands: []*Command{
			{Name: "streams", Description: "d", Subcommands: []*Command{
				{Name: "add", Description: "d", Options: []*Option{
					String("who", "d").Require(), Integer("n", "d"), Channel("in", "d"), Boolean("ping", "d"),
				}, Handler: func(ctx *Context) error {
					return ctx.Replyf("%s %d %s %t %s %s %s", ctx.Options.String("who"), ctx.Options.Int("n"),
						ctx.Options.ID("in"), ctx.Options.Bool("ping"), ctx.AuthorID(), ctx.GuildID(), ctx.ChannelID())
				}},
			}},
		}})

		run(gt.Command(7, "relay", gt.Group("streams", gt.Sub("add",
			gt.String("who", "pomu"), gt.Int("n", 3), gt.Channel("in", 42), gt.Bool("ping", true)))))

		gomega.Expect(rec.ResponseTypes()).To(gomega.Equal([]discord.InteractionResponseType{discord.InteractionResponseTypeCreateMessage}))
		gomega.Expect(rec.Responses()[0]["content"]).To(gomega.Equal("pomu 3 42 true 7 2000 3000"))
	})

	ginkgo.It("makes ephemeral commands private and edits the deferred reply", func() {
		r.MustRegister(&Command{Name: "slow", Description: "d", Ephemeral: true, Defer: true, Handler: func(ctx *Context) error {
			if err := ctx.Reply(Text("done")); err != nil {
				return err
			}
			_, err := ctx.Followup(Text("more"))
			return err
		}})

		run(gt.Command(7, "slow"))

		gomega.Expect(rec.ResponseTypes()).To(gomega.Equal([]discord.InteractionResponseType{discord.InteractionResponseTypeDeferredCreateMessage}))
		flags, _ := rec.Responses()[0]["flags"].(float64)
		gomega.Expect(discord.MessageFlags(flags).Has(discord.MessageFlagEphemeral)).To(gomega.BeTrue())

		edits := rec.Edits()
		gomega.Expect(edits).To(gomega.HaveLen(1))
		gomega.Expect(edits[0].Body["content"]).To(gomega.Equal("done"))
		gomega.Expect(edits[0].Body["embeds"]).To(gomega.Equal([]any{}))

		follow := rec.Followups()
		gomega.Expect(follow).To(gomega.HaveLen(1))
		gomega.Expect(follow[0].Path).To(gomega.HaveSuffix("/webhooks/" + gt.AppID.String() + "/token"))
		gomega.Expect(follow[0].Body["content"]).To(gomega.Equal("more"))
	})

	ginkgo.It("uploads files with the reply", func() {
		r.MustRegister(&Command{Name: "export", Description: "d", Handler: func(ctx *Context) error {
			return ctx.Reply(&Response{Content: "here", Files: []*discord.File{discord.NewFile("a.txt", "", strings.NewReader("data"))}})
		}})

		run(gt.Command(7, "export"))

		req := rec.Requests()[0]
		gomega.Expect(req.Files).To(gomega.Equal(map[string]string{"a.txt": "data"}))
		gomega.Expect(req.Body["data"]).To(gomega.HaveKeyWithValue("content", "here"))
	})

	ginkgo.It("answers user errors privately", func() {
		r.MustRegister(&Command{Name: "fail", Description: "d", Handler: func(*Context) error {
			return NewUserError("Nope.")
		}})

		run(gt.Command(7, "fail"))

		data := rec.Responses()[0]
		gomega.Expect(data["content"]).To(gomega.Equal("Nope."))
		gomega.Expect(discord.MessageFlags(data["flags"].(float64)).Has(discord.MessageFlagEphemeral)).To(gomega.BeTrue())
	})

	ginkgo.It("reports a missing subcommand", func() {
		var got error
		r = New(Config{ErrorHandler: func(_ *Context, err error) { got = err }})
		r.MustRegister(&Command{Name: "g", Description: "d", Subcommands: []*Command{{Name: "a", Description: "d", Handler: testOK}}})

		run(gt.Command(7, "g", gt.Sub("b")))

		gomega.Expect(got).To(gomega.MatchError(ErrUnknownSubcommand))
	})

	ginkgo.It("works in DMs", func() {
		var guild, channel snowflake.ID
		r.MustRegister(&Command{Name: "dm", Description: "d", Handler: func(ctx *Context) error {
			guild, channel = ctx.GuildID(), ctx.ChannelID()
			gomega.Expect(ctx.Member()).To(gomega.BeNil())
			gomega.Expect(ctx.AuthorID()).To(gomega.Equal(snowflake.ID(7)))
			p, err := ctx.Permissions()
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(p).To(gomega.Equal(discord.PermissionsAll))
			return nil
		}})

		run(gt.Command(7, "dm").InDM(55))

		gomega.Expect(guild).To(gomega.BeZero())
		gomega.Expect(channel).To(gomega.Equal(snowflake.ID(55)))
	})

	ginkgo.It("gives context menu commands their target", func() {
		var msg *discord.Message
		var user *discord.User
		var member *discord.Member
		r.MustRegister(
			&Command{Name: "Quote", Type: MessageContext, Handler: func(ctx *Context) error { msg = ctx.TargetMessage; return nil }},
			&Command{Name: "Profile", Type: UserContext, Handler: func(ctx *Context) error {
				user, member = ctx.TargetUser, ctx.TargetMember
				return nil
			}},
		)

		run(gt.MessageCommand(7, "Quote", 99, 8))
		run(gt.UserCommand(7, "Profile", 8))

		gomega.Expect(msg.ID).To(gomega.Equal(snowflake.ID(99)))
		gomega.Expect(msg.Author.ID).To(gomega.Equal(snowflake.ID(8)))
		gomega.Expect(user.ID).To(gomega.Equal(snowflake.ID(8)))
		gomega.Expect(member.User.ID).To(gomega.Equal(snowflake.ID(8)))
	})

	ginkgo.It("opens modals and reads what was submitted", func() {
		var submitted string
		cmd := &Command{Name: "settings", Description: "d", Handler: testOK}
		cmd.Components = func(ctx *ComponentContext) error {
			if ctx.IsModal() {
				submitted = ctx.TextInput("code")
				return ctx.Update(Text("saved"))
			}
			return ctx.Modal(ComponentID(cmd, "submit"), "Language",
				discord.NewLabel("Code", discord.NewShortTextInput("code")))
		}
		r.MustRegister(cmd)

		run(gt.Button(7, "rt:settings:open"))
		run(gt.ModalSubmit(7, "rt:settings:submit", map[string]string{"code": "FI"}))

		gomega.Expect(rec.ResponseTypes()).To(gomega.Equal([]discord.InteractionResponseType{
			discord.InteractionResponseTypeModal, discord.InteractionResponseTypeUpdateMessage,
		}))
		modal := rec.Responses()[0]
		gomega.Expect(modal["custom_id"]).To(gomega.Equal("rt:settings:submit"))
		label := modal["components"].([]any)[0].(map[string]any)
		gomega.Expect(label["label"]).To(gomega.Equal("Code"))
		gomega.Expect(submitted).To(gomega.Equal("FI"))
	})

	ginkgo.It("reads entity select menus as IDs", func() {
		var values []string
		r.MustRegister(&Command{Name: "settings", Description: "d", Handler: testOK, Components: func(ctx *ComponentContext) error {
			values = ctx.Values()
			return nil
		}})

		run(gt.SelectOf(discord.ComponentTypeRoleSelectMenu, 7, "rt:settings:roles", "11", "12"))

		gomega.Expect(values).To(gomega.Equal([]string{"11", "12"}))
	})
})

var _ = ginkgo.Describe("Syncing commands", func() {
	ginkgo.It("overwrites global and guild commands", func() {
		c, rec := gt.NewClient()
		perms := discord.PermissionManageGuild
		r := New(Config{})
		r.MustRegister(
			&Command{Name: "ping", Description: "d", Handler: testOK, DefaultMemberPermissions: &perms,
				Contexts: []discord.InteractionContextType{discord.InteractionContextTypeGuild},
				Options:  []*Option{Integer("n", "d").WithRange(1, 5).WithChoices(Choice{"one", 1})}},
			&Command{Name: "Find", Type: MessageContext, Handler: testOK},
			&Command{Name: "owner", Description: "d", Handler: testOK, GuildIDs: []snowflake.ID{9}},
		)

		gomega.Expect(r.Sync(c)).To(gomega.Succeed())

		reqs := rec.Requests()
		gomega.Expect(reqs).To(gomega.HaveLen(2))
		gomega.Expect(reqs[0].Method).To(gomega.Equal(http.MethodPut))
		gomega.Expect(reqs[0].Path).To(gomega.HaveSuffix("/applications/" + gt.AppID.String() + "/commands"))
		gomega.Expect(reqs[1].Path).To(gomega.HaveSuffix("/applications/" + gt.AppID.String() + "/guilds/9/commands"))
	})

	ginkgo.It("puts everything in the dev guild", func() {
		c, rec := gt.NewClient()
		r := New(Config{DevGuildID: 9})
		r.MustRegister(&Command{Name: "ping", Description: "d", Handler: testOK})

		gomega.Expect(r.Sync(c)).To(gomega.Succeed())

		reqs := rec.Requests()
		gomega.Expect(reqs).To(gomega.HaveLen(1))
		gomega.Expect(reqs[0].Path).To(gomega.HaveSuffix("/guilds/9/commands"))
	})

	ginkgo.It("builds Discord's command JSON", func() {
		perms := discord.PermissionManageGuild
		cmd := &Command{Name: "ping", Description: "d", Handler: testOK, DefaultMemberPermissions: &perms, NSFW: true,
			Options: []*Option{Integer("n", "d").WithRange(1, 5).WithChoices(Choice{"one", 1}), String("s", "d").WithLength(2, 3)}}
		gomega.Expect(cmd.validate(0)).To(gomega.Succeed())

		raw, err := cmd.applicationCommand().MarshalJSON()
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(raw).To(gomega.MatchJSON(`{
			"type": 1, "name": "ping", "description": "d", "default_member_permissions": "32", "nsfw": true,
			"options": [
				{"type": 4, "name": "n", "description": "d", "min_value": 1, "max_value": 5, "choices": [{"name": "one", "value": 1}]},
				{"type": 3, "name": "s", "description": "d", "min_length": 2, "max_length": 3}
			]
		}`))
	})
})

var _ = ginkgo.Describe("Running prefix commands", func() {
	ginkgo.It("replies to the message without the ephemeral flag", func() {
		c, rec := gt.NewClient()
		r := New(Config{Prefixes: []string{"!"}})
		r.MustRegister(&Command{Name: "ping", Description: "d", Ephemeral: true, Handler: func(ctx *Context) error {
			return ctx.Reply(Text("pong"))
		}})

		r.OnEvent(gt.Message(c, gt.GuildID, gt.ChannelID, 7, "!ping"))

		gomega.Expect(rec.Messages(gt.ChannelID)).To(gomega.Equal([]string{"pong"}))
		body := rec.Requests()[0].Body
		gomega.Expect(body).NotTo(gomega.HaveKey("flags"))
		gomega.Expect(body["message_reference"]).To(gomega.HaveKeyWithValue("message_id", "5000"))
	})

	ginkgo.It("checks permissions with the message's member when the author isn't cached", func() {
		c, rec := gt.NewClient()
		c.Caches.AddChannel(gt.GuildChannel(map[string]any{
			"id": gt.ChannelID.String(), "guild_id": gt.GuildID.String(), "type": discord.ChannelTypeGuildText,
		}))
		c.Caches.AddRole(discord.Role{ID: gt.GuildID, GuildID: gt.GuildID})
		c.Caches.AddRole(discord.Role{ID: 11, GuildID: gt.GuildID, Permissions: discord.PermissionManageGuild})

		r := New(Config{Prefixes: []string{"!"}})
		r.MustRegister(&Command{
			Name: "set", Description: "d",
			Checks:  []Check{HasPermissions(discord.PermissionManageGuild)},
			Handler: func(ctx *Context) error { return ctx.Reply(Text("ok")) },
		})

		mod := gt.Message(c, gt.GuildID, gt.ChannelID, 7, "!set")
		mod.Message.Member.RoleIDs = []snowflake.ID{11}
		r.OnEvent(mod)
		gomega.Expect(rec.Messages(gt.ChannelID)).To(gomega.Equal([]string{"ok"}))

		r.OnEvent(gt.Message(c, gt.GuildID, gt.ChannelID, 8, "!set"))
		gomega.Expect(rec.Messages(gt.ChannelID)).To(gomega.HaveLen(2))
		gomega.Expect(rec.Messages(gt.ChannelID)[1]).NotTo(gomega.Equal("ok"))
	})

	ginkgo.It("passes other messages to the fallback and ignores bots", func() {
		c, _ := gt.NewClient()
		var fell int
		r := New(Config{Prefixes: []string{"!"}, Fallback: func(*events.MessageCreate) { fell++ }})

		r.OnEvent(gt.Message(c, gt.GuildID, gt.ChannelID, 7, "hello"))
		bot := gt.Message(c, gt.GuildID, gt.ChannelID, 8, "hello")
		bot.Message.Author.Bot = true
		r.OnEvent(bot)

		gomega.Expect(fell).To(gomega.Equal(1))
	})
})
