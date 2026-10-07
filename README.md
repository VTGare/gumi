# gumi

A [DisGo](https://github.com/disgoorg/disgo) command framework. You declare a command once and
it runs as a slash command (or context menu command) and, optionally, as a classic prefixed message
command. Handlers read `ctx.Options` and never care how they were invoked.

```sh
go get github.com/VTGare/gumi/v2
```

## Example

```go
r := gumi.New(gumi.Config{
	Prefixes:              []string{"!"},
	DisablePrefixCommands: false, // set true for a slash-only bot
	OwnerIDs:              []snowflake.ID{123456789012345678},
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

client, err := disgo.New(token,
	bot.WithGatewayConfigOpts(gateway.WithIntents(gateway.IntentGuilds, gateway.IntentGuildMessages, gateway.IntentMessageContent)),
	// Handlers can take a while, so they must not block the gateway.
	bot.WithEventManagerConfigOpts(bot.WithAsyncEventsEnabled()),
	bot.WithEventListeners(r),
)
if err != nil {
	log.Fatal(err)
}

if err := client.OpenGateway(ctx); err != nil {
	log.Fatal(err)
}
if err := r.Sync(client); err != nil {
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
- `gumitest` builds interactions and messages the way Discord sends them and records REST calls,
  so command tests run without Discord.

## Notes for DisGo

- IDs are `snowflake.ID`; `GuildID()` is 0 outside guilds.
- Enable async events, or a slow handler blocks the gateway.
- Prefix commands read permissions, channels and members from the client's caches. Turn on the
  cache flags you need (`cache.FlagMembers`, `cache.FlagChannels`, `cache.FlagRoles`).
- Modal text inputs go in a `discord.LabelComponent`, which carries the label.
