package utils

import (
	"strings"

	"github.com/bwmarrin/discordgo"
)

func HasPrefixes(str string, prefixes []string, caseSensitive bool) (bool, string) {
	for _, prefix := range prefixes {
		stringToCheck := str
		if !caseSensitive {
			stringToCheck = strings.ToLower(stringToCheck)
			prefix = strings.ToLower(prefix)
		}
		if strings.HasPrefix(stringToCheck, prefix) {
			return true, string(str[len(prefix):])
		}
	}
	return false, str
}

//MemberHasPermission checks if guild member has a permission to do something on a server.
func MemberHasPermission(s *discordgo.Session, guildID string, userID string, permission int64) (bool, error) {
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

func MapString(ss []string, f func(string) string) []string {
	mapped := make([]string, 0, len(ss))
	for _, s := range ss {
		mapped = append(mapped, f(s))
	}

	return mapped
}
