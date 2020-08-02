package gumi

import (
	"errors"
	"fmt"

	"github.com/VTGare/gumi/utils"
	"github.com/bwmarrin/discordgo"
)

func defaultHelp(g *Gumi, s *discordgo.Session, m *discordgo.MessageCreate, args []string) *discordgo.MessageSend {
	prefix := g.PrefixHandler(g, s, m)

	embed := &discordgo.MessageEmbed{
		Description: fmt.Sprintf("Use ``%vhelp <group name> <command name>`` for extended help on specific commands.", prefix[0]),
		Color:       utils.EmbedColor,
		Timestamp:   utils.EmbedTimestamp(),
		Thumbnail: &discordgo.MessageEmbedThumbnail{
			URL: utils.EmbedImage,
		},
	}

	switch len(args) {
	case 0:
		embed.Title = "Help"
		for _, group := range g.Groups {
			if group.IsVisible {
				field := &discordgo.MessageEmbedField{
					Name:  group.Name,
					Value: group.Description,
				}
				embed.Fields = append(embed.Fields, field)
			}
		}
	case 1:
		if group, ok := g.Groups[args[0]]; ok {
			embed.Title = fmt.Sprintf("%v group command list", args[0])

			used := map[string]bool{}
			for _, command := range group.Commands {
				_, ok := used[command.Name]
				if command.Help.IsVisible && !ok {
					field := &discordgo.MessageEmbedField{
						Name:  command.Name,
						Value: command.createHelp(),
					}
					used[command.Name] = true
					embed.Fields = append(embed.Fields, field)
				}
			}
		} else {
			return g.ErrorHandler(fmt.Errorf("unknown group %v", args[0]))
		}
	case 2:
		if group, ok := g.Groups[args[0]]; ok {
			if command, ok := group.Commands[args[1]]; ok {
				if command.Help.IsVisible && command.Help.ExtendedHelp != nil {
					embed.Title = fmt.Sprintf("%v command extended help", command.Name)
					embed.Fields = command.Help.ExtendedHelp
				} else {
					return g.ErrorHandler(fmt.Errorf("command %v is invisible or doesn't have extended help", args[0]))
				}
			} else {
				return g.ErrorHandler(fmt.Errorf("unknown command %v", args[1]))
			}
		} else {
			return g.ErrorHandler(fmt.Errorf("unknown group %v", args[0]))
		}
	default:
		return g.ErrorHandler(errors.New("incorrect command usage. Example: bt!help <group> <command name>"))
	}

	return &discordgo.MessageSend{
		Embed: embed,
	}
}

func defaultError(e error) *discordgo.MessageSend {
	if e != nil {
		embed := &discordgo.MessageEmbed{
			Title: "Oops, something went wrong!",
			Thumbnail: &discordgo.MessageEmbedThumbnail{
				URL: utils.EmbedImage,
			},
			Description: fmt.Sprintf("***Error message:***\n%v", e),
			Color:       utils.EmbedColor,
			Timestamp:   utils.EmbedTimestamp(),
		}

		return &discordgo.MessageSend{
			Embed: embed,
		}
	}
	return nil
}
