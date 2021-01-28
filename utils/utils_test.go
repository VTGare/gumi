package utils

import "testing"

func TestHasPrefixes(t *testing.T) {
	type args struct {
		s             string
		prefixes      []string
		caseSensitive bool
	}
	tests := []struct {
		name  string
		args  args
		want  bool
		want1 string
	}{
		{
			"1 prefix, case sensitive",
			args{"bt command", []string{"bt "}, true},
			true,
			"command",
		},
		{
			"1 prefix, case sensitive fail",
			args{"Bt command", []string{"bt "}, true},
			false,
			"Bt command",
		},
		{
			"1 prefix, case insensitive",
			args{"Bt command", []string{"bt "}, false},
			true,
			"command",
		},
		{
			"1 prefix, case insensitive fail",
			args{"Bt command", []string{"b."}, false},
			false,
			"Bt command",
		},
		{
			"2 prefixes, case sensitive",
			args{"bt!command", []string{"bt ", "bt!"}, true},
			true,
			"command",
		},
		{
			"2 prefixes, case sensitive fail",
			args{"Bt!command", []string{"bt ", "bt!"}, true},
			false,
			"Bt!command",
		},
		{
			"2 prefixes, case insensitive",
			args{"Bt!command", []string{"bt ", "bt!"}, false},
			true,
			"command",
		},
		{
			"2 prefixes, case insensitive fail",
			args{"t!command", []string{"bt ", "bt!"}, false},
			false,
			"t!command",
		},
		{
			"space",
			args{"bt command", []string{"bt "}, true},
			true,
			"command",
		},
		{
			"case insensitive two arguments",
			args{"BT!COMMAND Amelia Watson", []string{"bt!"}, false},
			true,
			"COMMAND Amelia Watson",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1 := HasPrefixes(tt.args.s, tt.args.prefixes, tt.args.caseSensitive)
			if got != tt.want {
				t.Errorf("HasPrefixes() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("HasPrefixes() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}
