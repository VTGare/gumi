package utils

import "testing"

func TestHasPrefixes(t *testing.T) {
	type args struct {
		s        string
		prefixes []string
	}
	tests := []struct {
		name  string
		args  args
		want  bool
		want1 string
	}{
		{
			"1 prefix",
			args{"!command", []string{"!"}},
			true,
			"command",
		},
		{
			"2 prefixes",
			args{">command", []string{"!", ">"}},
			true,
			"command",
		},
		{
			"fail",
			args{"!command", []string{">"}},
			false,
			"!command",
		},
		{
			"space",
			args{"bt command", []string{"bt "}},
			true,
			"command",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1 := HasPrefixes(tt.args.s, tt.args.prefixes)
			if got != tt.want {
				t.Errorf("HasPrefixes() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("HasPrefixes() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}
