# gumi

A [DiscordGo](https://github.com/bwmarrin/discordgo) command framework. You declare a command once and
it runs as a slash command (or context menu command) and, optionally, as a classic prefixed message
command. Handlers read `ctx.Options` and never care how they were invoked.

```sh
go get github.com/VTGare/gumi
```

## Example

```go
r := gumi.New(gumi.Config{
	Prefixes:              []string{"!"},
	DisablePrefixCommands: false, // set true for a slash-only bot
	OwnerIDs:              []string{"123456789012345678"},
})

r.Use(
	middleware.Logging(slog.Default()),
	middleware.Recover(),
	middleware.Timeout(time.Minute),
)

r.MustRegister(&gumi.Command{
	Name:        "greet",
	Description: "Say hello",
	Options: []*gumi.Option{
		gumi.String("name", "Who to greet").Require().WithAutocomplete(
			func(ctx *gumi.AutocompleteContext) ([]gumi.Choice, error) {
				return []gumi.Choice{{Name: "World", Value: "World"}}, nil
			},
		),
	},
	Handler: func(ctx *gumi.Context) error {
		return ctx.Replyf("Hello, %s!", ctx.Options.String("name"))
	},
})

unbind := r.Bind(session) // after discordgo.New
defer unbind()

// After session.Open():
if err := r.Sync(session); err != nil {
	log.Fatal(err)
}
```

## Features

- Slash, message context and user context commands. Prefix invocation can be turned off.
- Subcommands and groups (two levels deep), with a default subcommand for prefix invocations.
- Typed options with choices, ranges, lengths, channel types, greedy strings and slash-only options.
- **Autocomplete** for string, integer and number options. The command's checks gate suggestions.
- Middleware (`middleware.Logging`, `Recover`, `Timeout`), checks (`GuildOnly`, `OwnerOnly`,
  `HasPermissions`, `BotHasPermissions`, `NSFW`, `Any`, `Not`) and cooldowns.
- Stateless components and modals that route back to their command via `ComponentID`.
- Built-in help command.
- One reply API for both invocation kinds: `Reply`, `Defer`, `Edit`, `Followup`, ephemeral replies.
- `Sync` bulk-overwrites application commands, globally or to a development guild.

## History

v0.x was the original 2020–2021 framework. v1 is a rewrite that grew out of
[boe-tea-go](https://github.com/VTGare/boe-tea-go)'s router.
