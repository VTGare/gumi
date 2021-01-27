package gumi

import (
	"github.com/VTGare/gumi/utils"
)

type Command struct {
	Name        string
	Group       string
	Aliases     []string
	Description string
	Usage       string
	Example     string
	Flags       map[string]string
	GuildOnly   bool
	NSFW        bool
	Permissions int
	RateLimiter *RateLimiter
	Exec        ExecutionHandler
}

func (c *Command) execute(ctx *Ctx) error {
	if callback := ctx.Router.OnExecuteCallback; callback != nil {
		err := callback(ctx)
		if err != nil {
			return err
		}
	}

	//check permissions to execute a command
	if c.Permissions != 0 {
		hasPerms, err := utils.MemberHasPermission(ctx.Session, ctx.Event.GuildID, ctx.Event.Author.ID, c.Permissions)
		if err != nil {
			return err
		}

		if !hasPerms {
			if callback := ctx.Router.OnNoPermissionsCallback; callback != nil {
				err := callback(ctx)
				if err != nil {
					return err
				}
			}

			return nil
		}
	}

	if c.GuildOnly && ctx.Event.GuildID == "" {
		return nil
	}

	if c.NSFW {
		ch, err := ctx.Session.Channel(ctx.Event.ChannelID)
		if err != nil {
			return err
		}

		if !ch.NSFW {
			if callback := ctx.Router.OnNSFWCallback; callback != nil {
				err := callback(ctx)
				if err != nil {
					return err
				}
			}

			return nil
		}
	}

	if c.RateLimiter != nil {
		if c.RateLimiter.Contains(ctx.Event.Author.ID) {
			return ctx.Router.OnRateLimitCallback(ctx)
		}

		c.RateLimiter.Set(ctx.Event.Author.ID)
	}

	return c.Exec(ctx)
}
