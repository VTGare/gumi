package gumi

import (
	"strings"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"

	gt "github.com/VTGare/gumi/v2/gumitest"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
)

var _ = ginkgo.Describe("Component IDs", func() {
	ginkgo.It("routes to the root command with args", func() {
		root := &Command{Name: "settings"}
		sub := &Command{Name: "sub", parent: root}

		gomega.Expect(ComponentID(root, "u", "toggle", "tags")).To(gomega.Equal("rt:settings:u:toggle:tags"))
		gomega.Expect(ComponentID(sub, "x")).To(gomega.Equal("rt:settings:x"))
		gomega.Expect(ComponentID(root)).To(gomega.Equal("rt:settings"))
	})

	ginkgo.It("refuses IDs over Discord's limit", func() {
		gomega.Expect(ComponentID(&Command{Name: "settings"}, strings.Repeat("x", 100))).To(gomega.BeEmpty())
	})
})

var _ = ginkgo.Describe("Component contexts", func() {
	ginkgo.It("reads args, values and the user", func() {
		ctx := &ComponentContext{
			Args:        []string{"u", "nav"},
			Interaction: gt.Select(7, "rt:settings:u:nav", "posting").WithPermissions(discord.PermissionManageGuild).Parse(),
		}

		gomega.Expect(ctx.Arg(1)).To(gomega.Equal("nav"))
		gomega.Expect(ctx.Arg(5)).To(gomega.BeEmpty())
		gomega.Expect(ctx.Values()).To(gomega.Equal([]string{"posting"}))
		gomega.Expect(ctx.UserID()).To(gomega.Equal(snowflake.ID(7)))
		gomega.Expect(ctx.Permissions()).To(gomega.Equal(discord.PermissionManageGuild))
		gomega.Expect(ctx.IsModal()).To(gomega.BeFalse())
	})

	ginkgo.It("reads modal text inputs", func() {
		ctx := &ComponentContext{Interaction: gt.ModalSubmit(7, "rt:settings:u:submit:limit", map[string]string{"value": "20"}).Parse()}

		gomega.Expect(ctx.IsModal()).To(gomega.BeTrue())
		gomega.Expect(ctx.TextInput("value")).To(gomega.Equal("20"))
		gomega.Expect(ctx.TextInput("other")).To(gomega.BeEmpty())
		gomega.Expect(ctx.Values()).To(gomega.BeNil())
	})
})

var _ = ginkgo.Describe("Routing components", func() {
	c, _ := gt.NewClient()
	click := func(r *Router, customID string) { r.HandleInteraction(gt.Button(7, customID).Event(c)) }

	ginkgo.It("hands the click to the owning command with its args", func() {
		var got []string
		r := New(Config{})
		r.MustRegister(&Command{
			Name: "settings", Description: "d", Handler: testOK,
			Components: func(ctx *ComponentContext) error {
				got = ctx.Args
				return nil
			},
		})

		click(r, "rt:settings:u:toggle:tags")
		gomega.Expect(got).To(gomega.Equal([]string{"u", "toggle", "tags"}))
	})

	ginkgo.It("ignores IDs it does not own", func() {
		called := false
		r := New(Config{})
		r.MustRegister(&Command{
			Name: "settings", Description: "d", Handler: testOK,
			Components: func(*ComponentContext) error { called = true; return nil },
		})

		click(r, "boe:page:1")
		click(r, "rt:other:u")
		gomega.Expect(called).To(gomega.BeFalse())
	})
})
