package gumi

import (
	"reflect"
	"testing"
)

func TestParseArguments(t *testing.T) {
	type args struct {
		raw string
	}
	tests := []struct {
		name string
		args args
		want *Arguments
	}{
		{
			"1",
			args{"argument1 argument2"},
			&Arguments{Raw: "argument1 argument2", Arguments: []*Argument{
				{"argument1"},
				{"argument2"},
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ParseArguments(tt.args.raw); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseArguments() = %v, want %v", got, tt.want)
			}
		})
	}
}
