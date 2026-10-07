package middleware_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"

	"github.com/VTGare/gumi/v2"
	"github.com/VTGare/gumi/v2/middleware"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Logging", func() {
	run := func(err error) map[string]any {
		var buf bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&buf, nil))

		guildID := snowflake.ID(1)
		ctx := &gumi.Context{
			Command: &gumi.Command{Name: "ping"},
			Message: &discord.Message{GuildID: &guildID, ChannelID: 2, Author: discord.User{ID: 3, Username: "vt"}},
		}

		got := middleware.Logging(logger)(func(*gumi.Context) error { return err })(ctx)
		if err == nil {
			Expect(got).NotTo(HaveOccurred())
		} else {
			Expect(got).To(MatchError(err))
		}

		var entry map[string]any
		Expect(json.Unmarshal(buf.Bytes(), &entry)).To(Succeed())
		return entry
	}

	It("logs successful commands at info with invocation fields", func() {
		entry := run(nil)

		Expect(entry).To(HaveKeyWithValue("level", "INFO"))
		Expect(entry).To(HaveKeyWithValue("msg", "command executed"))
		Expect(entry).To(HaveKeyWithValue("command", "ping"))
		Expect(entry).To(HaveKeyWithValue("source", "message"))
		Expect(entry).To(HaveKeyWithValue("user_id", "3"))
		Expect(entry).To(HaveKeyWithValue("guild_id", "1"))
		Expect(entry).To(HaveKeyWithValue("channel_id", "2"))
		Expect(entry).To(HaveKeyWithValue("user", "vt"))
		Expect(entry).To(HaveKey("duration"))
	})

	It("logs user-facing rejections at info with the reason", func() {
		entry := run(gumi.Errorf("Command not found."))

		Expect(entry).To(HaveKeyWithValue("level", "INFO"))
		Expect(entry).To(HaveKeyWithValue("msg", "command rejected"))
		Expect(entry).To(HaveKeyWithValue("reason", "Command not found."))
	})

	It("logs other failures at error", func() {
		entry := run(errors.New("boom"))

		Expect(entry).To(HaveKeyWithValue("level", "ERROR"))
		Expect(entry).To(HaveKeyWithValue("msg", "command failed"))
		Expect(entry).To(HaveKeyWithValue("error", "boom"))
	})

	It("discards everything with a nil logger", func() {
		h := middleware.Logging(nil)(func(*gumi.Context) error { return nil })
		Expect(h(&gumi.Context{Command: &gumi.Command{Name: "ping"}})).To(Succeed())
	})
})
