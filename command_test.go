package gumi

import (
	"reflect"
	"testing"
	"time"
)

func TestCommandOnCooldown(t *testing.T) {
	type args struct {
		id string
	}
	tests := []struct {
		name string
		c    *Command
		args args
		want time.Duration
	}{
		{"1", &Command{execMap: map[string]time.Time{"1": time.Now()}, Cooldown: 0}, args{"1"}, 0},
		{"2", &Command{execMap: map[string]time.Time{"1": time.Now()}, Cooldown: 0}, args{"0"}, 0},
		{"3", &Command{execMap: map[string]time.Time{"1": time.Now()}, Cooldown: 5 * time.Second}, args{"1"}, 5 * time.Second},
		{"4", &Command{execMap: map[string]time.Time{"1": time.Now().Add(-time.Second)}, Cooldown: 5 * time.Second}, args{"1"}, 4 * time.Second},
		{"5", &Command{execMap: map[string]time.Time{"1": time.Now().Add(-2 * time.Second)}, Cooldown: 5 * time.Second}, args{"1"}, 3 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.c.onCooldown(tt.args.id); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Command.onCooldown() = %v, want %v", got, tt.want)
			}
		})
	}
}
