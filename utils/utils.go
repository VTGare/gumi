package utils

import "time"

var (
	//EmbedColor is a default border colour for Discord embeds
	EmbedColor = 0x439ef1
	EmbedImage = "https://i.imgur.com/OZ1Al5h.png"
)

//EmbedTimestamp returns currect time formatted to RFC3339 for Discord Embeds
func EmbedTimestamp() string {
	return time.Now().Format(time.RFC3339)
}

//MapString ...
func MapString(vs []string, f func(string) string) []string {
	vsm := make([]string, len(vs))
	for i, v := range vs {
		vsm[i] = f(v)
	}
	return vsm
}
