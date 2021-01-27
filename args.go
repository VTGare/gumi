package gumi

import (
	"regexp"
	"strconv"
	"strings"
)

var (
	// RegexUserMention defines the regex a user mention has to match
	RegexUserMention = regexp.MustCompile("<@!?(\\d+)>")

	// RegexRoleMention defines the regex a role mention has to match
	RegexRoleMention = regexp.MustCompile("<@&(\\d+)>")

	// RegexChannelMention defines the regex a channel mention has to match
	RegexChannelMention = regexp.MustCompile("<#(\\d+)>")
)

//Arguments represents arguments used in a command.
type Arguments struct {
	Raw       string
	Arguments []*Argument
}

//Argument represents a single argument
type Argument struct {
	Raw string
}

func ParseArguments(raw string) *Arguments {
	fields := strings.Fields(raw)

	args := make([]*Argument, 0, len(fields))
	for _, f := range fields {
		args = append(args, &Argument{f})
	}

	return &Arguments{
		Arguments: args,
		Raw:       raw,
	}
}

// AsSingle parses the given arguments as a single one
func (arguments *Arguments) AsSingle() *Argument {
	return &Argument{
		Raw: arguments.Raw,
	}
}

// Len returns the length of the arguments
func (arguments *Arguments) Len() int {
	return len(arguments.Arguments)
}

// Get returns the n'th argument
func (arguments *Arguments) Get(n int) *Argument {
	if arguments.Len() <= n {
		return &Argument{
			Raw: "",
		}
	}
	return arguments.Arguments[n]
}

// Remove removes the n'th argument
func (arguments *Arguments) Remove(n int) {
	// Check if the given index is valid
	if arguments.Len() <= n {
		return
	}

	// Set the new argument slice
	arguments.Arguments = append(arguments.Arguments[:n], arguments.Arguments[n+1:]...)

	// Set the new raw string
	raw := ""
	for _, argument := range arguments.Arguments {
		raw += argument.Raw + " "
	}
	arguments.Raw = strings.TrimSpace(raw)
}

// AsBool parses the given argument into a boolean
func (argument *Argument) AsBool() (bool, error) {
	return strconv.ParseBool(argument.Raw)
}

// AsInt parses the given argument into an int32
func (argument *Argument) AsInt() (int, error) {
	return strconv.Atoi(argument.Raw)
}

// AsInt64 parses the given argument into an int64
func (argument *Argument) AsInt64() (int64, error) {
	return strconv.ParseInt(argument.Raw, 10, 64)
}

// AsUserMentionID returns the ID of the mentioned user or an empty string if it is no mention
func (argument *Argument) AsUserMentionID() string {
	// Check if the argument is a user mention
	matches := RegexUserMention.MatchString(argument.Raw)
	if !matches {
		return ""
	}

	// Parse the user ID
	userID := RegexUserMention.FindStringSubmatch(argument.Raw)[1]
	return userID
}

// AsRoleMentionID returns the ID of the mentioned role or an empty string if it is no mention
func (argument *Argument) AsRoleMentionID() string {
	// Check if the argument is a role mention
	matches := RegexRoleMention.MatchString(argument.Raw)
	if !matches {
		return ""
	}

	// Parse the role ID
	roleID := RegexRoleMention.FindStringSubmatch(argument.Raw)[1]
	return roleID
}

// AsChannelMentionID returns the ID of the mentioned channel or an empty string if it is no mention
func (argument *Argument) AsChannelMentionID() string {
	// Check if the argument is a channel mention
	matches := RegexChannelMention.MatchString(argument.Raw)
	if !matches {
		return ""
	}

	// Parse the channel ID
	channelID := RegexChannelMention.FindStringSubmatch(argument.Raw)[1]
	return channelID
}
