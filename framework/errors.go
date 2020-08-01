package gumi

import "fmt"

func errNotGuild(command string) error {
	return fmt.Errorf("command %v cannot be executed in direct messages", command)
}
