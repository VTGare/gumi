package gumi

import "github.com/bwmarrin/discordgo"

type Ctx struct {
	Session *discordgo.Session
	Event   *discordgo.MessageCreate
	Args    *Arguments
	Router  *Router
	Command *Command
}

// ExecutionHandler represents a handler for a context execution
type ExecutionHandler func(*Ctx) error

// Reply responds with the given text message
func (ctx *Ctx) Reply(text string) error {
	_, err := ctx.Session.ChannelMessageSend(ctx.Event.ChannelID, text)
	return err
}

// ReplyEmbed responds with the given embed message
func (ctx *Ctx) ReplyEmbed(embed *discordgo.MessageEmbed) error {
	_, err := ctx.Session.ChannelMessageSendEmbed(ctx.Event.ChannelID, embed)
	return err
}

// ReplyTextEmbed responds with the given text and embed message
func (ctx *Ctx) ReplyTextEmbed(text string, embed *discordgo.MessageEmbed) error {
	_, err := ctx.Session.ChannelMessageSendComplex(ctx.Event.ChannelID, &discordgo.MessageSend{
		Content: text,
		Embed:   embed,
	})
	return err
}
