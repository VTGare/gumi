package gumi

import (
	"strings"
	"testing"

	"github.com/bwmarrin/discordgo"
)

func TestInit(t *testing.T) {
	g := NewGumi()
	if general, ok := g.Groups["general"]; ok {
		_, help := general.Commands["help"]
		if !help {
			t.Fatal("help command not initialized")
		}
	} else {
		t.Fatal("general group not initialized")
	}
}

func TestTrim(t *testing.T) {
	g := NewGumi()
	content, trimmed := g.trimPrefix(&discordgo.Session{}, &discordgo.MessageCreate{}, "?test")
	if strings.HasPrefix(content, "?") && trimmed {
		t.Fatal("failed to detect trim")
	} else if strings.HasPrefix(content, "?") {
		t.Fatal("failed to trim default prefix")
	}

	g.DefaultPrefixes = []string{"str!", "str?"}
	content, trimmed = g.trimPrefix(&discordgo.Session{}, &discordgo.MessageCreate{}, "str?test")
	if strings.HasPrefix(content, "str?") {
		t.Fatal("failed to trim second prefix")
	}
}

func TestAddGroup(t *testing.T) {
	g := NewGumi()
	g.AddGroup("test")
	if _, ok := g.Groups["test"]; !ok {
		t.Fatal("group not added")
	}

	test := g.AddGroup("test2", GroupDescription("test"), GroupNSFW())
	if test.Description != "test" {
		t.Fatal("description hasnt changed")
	}

	if !test.NSFW {
		t.Fatal("NSFW hasnt changed")
	}
}

func TestCommand(t *testing.T) {
	g := NewGumi()
	tg := g.AddGroup("test")
	cmd := tg.AddCommand("test", func(s *discordgo.Session, m *discordgo.MessageCreate, args []string) error {
		return nil
	}, WithAliases("test1", "test2"))

	if _, ok := tg.Commands["test1"]; !ok {
		t.Fatal("alias not added")
	}

	if cmd.Name != "test" {
		t.Fatal("name incorrect")
	}
}
