package gumi

import (
	"github.com/disgoorg/disgo/discord"
)

// GuildOnly rejects invocations outside of guilds.
var GuildOnly Check = func(ctx *Context) error {
	if ctx.GuildID() == 0 {
		return &CheckError{Check: "guild_only", Message: "This command can only be used in a server."}
	}

	return nil
}

// DMOnly rejects invocations inside guilds.
var DMOnly Check = func(ctx *Context) error {
	if ctx.GuildID() != 0 {
		return &CheckError{Check: "dm_only", Message: "This command can only be used in DMs."}
	}

	return nil
}

// NSFW rejects invocations in non-NSFW guild channels. DMs are allowed.
// Threads inherit the flag from their parent channel.
var NSFW Check = func(ctx *Context) error {
	if ctx.GuildID() == 0 {
		return nil
	}

	ch, err := ctx.Channel()
	if err != nil {
		return err
	}
	if th, ok := ch.(discord.GuildThread); ok && th.ParentID() != nil {
		if ch, err = ctx.channel(*th.ParentID()); err != nil {
			return err
		}
	}
	if mc, ok := ch.(discord.GuildMessageChannel); !ok || !mc.NSFW() {
		return &CheckError{Check: "nsfw", Message: "This command can only be used in NSFW channels."}
	}
	return nil
}

// OwnerOnly silently rejects everyone except Config.OwnerIDs.
var OwnerOnly Check = func(ctx *Context) error {
	if !ctx.Router.IsOwner(ctx.AuthorID()) {
		return &CheckError{Check: "owner_only", Message: "This command is restricted to the bot owner.", Silent: true}
	}

	return nil
}

// HasPermissions needs all the given channel permissions (admins pass).
// Outside guilds it passes.
func HasPermissions(permissions discord.Permissions) Check {
	return permissionCheck(permissions, (*Context).Permissions,
		"permissions", "You don't have permission to use this command.")
}

// BotHasPermissions needs all the given channel permissions for the bot.
// Outside guilds it passes.
func BotHasPermissions(permissions discord.Permissions) Check {
	return permissionCheck(permissions, (*Context).BotPermissions,
		"bot_permissions", "I'm missing the permissions required to run this command.")
}

func permissionCheck(permissions discord.Permissions, get func(*Context) (discord.Permissions, error), name, message string) Check {
	return func(ctx *Context) error {
		if ctx.GuildID() == 0 {
			return nil
		}

		p, err := get(ctx)
		if err != nil {
			return err
		}

		if p.Has(discord.PermissionAdministrator) || p.Has(permissions) {
			return nil
		}

		return &CheckError{Check: name, Message: message}
	}
}

// Any passes if at least one of the checks passes. If all fail, the error of
// the last check is returned.
func Any(checks ...Check) Check {
	return func(ctx *Context) error {
		var last error

		for _, chk := range checks {
			err := chk(ctx)
			if err == nil {
				return nil
			}

			last = err
		}

		return last
	}
}

// Not inverts a check, using message as the rejection reason.
func Not(check Check, message string) Check {
	return func(ctx *Context) error {
		if err := check(ctx); err != nil {
			return nil
		}

		return &CheckError{Check: "not", Message: message}
	}
}
