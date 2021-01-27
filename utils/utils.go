package utils

import (
	"strings"

	"github.com/bwmarrin/discordgo"
)

func HasPrefixes(s string, prefixes []string) (bool, string) {
	for _, prefix := range prefixes {
		if strings.HasPrefix(s, prefix) {
			return true, strings.TrimPrefix(s, prefix)
		}
	}

	return false, s
}

//MemberHasPermission checks if guild member has a permission to do something on a server.
func MemberHasPermission(s *discordgo.Session, guildID string, userID string, permission int) (bool, error) {
	member, err := s.State.Member(guildID, userID)
	if err != nil {
		if member, err = s.GuildMember(guildID, userID); err != nil {
			return false, err
		}
	}
	g, err := s.Guild(guildID)
	if err != nil {
		return false, err
	}

	if g.OwnerID == userID {
		return true, nil
	}
	// Iterate through the role IDs stored in member.Roles
	// to check permissions
	for _, roleID := range member.Roles {
		role, err := s.State.Role(guildID, roleID)
		if err != nil {
			return false, err
		}
		if role.Permissions&permission != 0 {
			return true, nil
		}
	}

	return false, nil
}
