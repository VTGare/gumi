// Package gumitest drives a gumi router in tests. DisGo's interactions
// have unexported fields, so the builders write the JSON Discord would send
// and parse it. A Recorder stands in for Discord's HTTP API.
package gumitest

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"
	"sync"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/cache"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"
)

// The IDs every builder uses unless told otherwise.
const (
	AppID     snowflake.ID = 1000
	BotID     snowflake.ID = 1001
	GuildID   snowflake.ID = 2000
	ChannelID snowflake.ID = 3000
)

// disgo.New reads AppID out of it.
const Token = "MTAwMA.test.test"

const Timestamp = "2024-01-01T00:00:00Z"

type Request struct {
	Method string
	Path   string
	Body   map[string]any
	// Uploaded files by name, for multipart requests.
	Files map[string]string
}

// Answers interaction callbacks with 204, command overwrites with [] and
// everything else with {}.
type Recorder struct {
	mu   sync.Mutex
	reqs []Request
}

func (r *Recorder) RoundTrip(req *http.Request) (*http.Response, error) {
	rec := Request{Method: req.Method, Path: req.URL.Path}
	if req.Body != nil {
		rec.Body, rec.Files = readBody(req)
	}

	r.mu.Lock()
	r.reqs = append(r.reqs, rec)
	r.mu.Unlock()

	status, body := http.StatusOK, "{}"
	switch {
	case strings.HasSuffix(req.URL.Path, "/callback"):
		status, body = http.StatusNoContent, ""
	case req.Method == http.MethodPut && strings.HasSuffix(req.URL.Path, "/commands"):
		body = "[]"
	}

	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}, nil
}

// With files, DisGo sends multipart bodies with the JSON in a payload_json
// part.
func readBody(req *http.Request) (map[string]any, map[string]string) {
	var body map[string]any

	mediaType, params, _ := mime.ParseMediaType(req.Header.Get("Content-Type"))
	if !strings.HasPrefix(mediaType, "multipart/") {
		raw, _ := io.ReadAll(req.Body)
		_ = json.Unmarshal(raw, &body)
		return body, nil
	}

	files := map[string]string{}
	mr := multipart.NewReader(req.Body, params["boundary"])
	for {
		part, err := mr.NextPart()
		if err != nil {
			break
		}

		raw, _ := io.ReadAll(part)
		if part.FormName() == "payload_json" {
			_ = json.Unmarshal(raw, &body)
		} else if part.FileName() != "" {
			files[part.FileName()] = string(raw)
		}
	}

	return body, files
}

func (r *Recorder) Requests() []Request {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Request(nil), r.reqs...)
}

// Only the contents, in the order they were posted.
func (r *Recorder) Messages(channelID snowflake.ID) []string {
	var out []string
	for _, req := range r.Requests() {
		if req.Method == http.MethodPost && strings.HasSuffix(req.Path, "/channels/"+channelID.String()+"/messages") {
			content, _ := req.Body["content"].(string)
			out = append(out, content)
		}
	}
	return out
}

// Only initial responses. Deferred replies arrive as Edits.
func (r *Recorder) Responses() []map[string]any {
	var out []map[string]any
	for _, req := range r.Requests() {
		if strings.HasSuffix(req.Path, "/callback") {
			data, _ := req.Body["data"].(map[string]any)
			out = append(out, data)
		}
	}
	return out
}

func (r *Recorder) ResponseTypes() []discord.InteractionResponseType {
	var out []discord.InteractionResponseType
	for _, req := range r.Requests() {
		if strings.HasSuffix(req.Path, "/callback") {
			t, _ := req.Body["type"].(float64)
			out = append(out, discord.InteractionResponseType(t))
		}
	}
	return out
}

func (r *Recorder) Edits() []Request {
	var out []Request
	for _, req := range r.Requests() {
		if req.Method == http.MethodPatch && strings.HasSuffix(req.Path, "/messages/@original") {
			out = append(out, req)
		}
	}
	return out
}

func (r *Recorder) Followups() []Request {
	var out []Request
	for _, req := range r.Requests() {
		if req.Method == http.MethodPost && strings.Contains(req.Path, "/webhooks/") {
			out = append(out, req)
		}
	}
	return out
}

// The client has no gateway. Its REST calls go to the returned Recorder,
// and its caches are on and know the bot user.
func NewClient() (*bot.Client, *Recorder) {
	rec := &Recorder{}
	logger := slog.New(slog.DiscardHandler)

	caches := cache.New(cache.WithCaches(cache.FlagsAll))
	caches.SetSelfUser(discord.OAuth2User{User: discord.User{ID: BotID, Username: "bot", Bot: true}})

	c := &bot.Client{
		Token:         "test",
		ApplicationID: AppID,
		Logger:        logger,
		Rest: rest.New(rest.NewClient("test",
			rest.WithHTTPClient(&http.Client{Transport: rec}),
			rest.WithRateLimiter(rest.NewNoopRateLimiter()),
			rest.WithLogger(logger),
		)),
		Caches: caches,
	}
	return c, rec
}

func Attach(c *bot.Client) *Recorder {
	rec := &Recorder{}
	c.Rest.HTTPClient().Transport = rec
	return rec
}

type Option map[string]any

func Sub(name string, opts ...Option) Option {
	return Option{"name": name, "type": discord.ApplicationCommandOptionTypeSubCommand, "options": options(opts)}
}

func Group(name string, opts ...Option) Option {
	return Option{"name": name, "type": discord.ApplicationCommandOptionTypeSubCommandGroup, "options": options(opts)}
}

func String(name, value string) Option {
	return Option{"name": name, "type": discord.ApplicationCommandOptionTypeString, "value": value}
}

func Int(name string, value int) Option {
	return Option{"name": name, "type": discord.ApplicationCommandOptionTypeInt, "value": value}
}

func Float(name string, value float64) Option {
	return Option{"name": name, "type": discord.ApplicationCommandOptionTypeFloat, "value": value}
}

func Bool(name string, value bool) Option {
	return Option{"name": name, "type": discord.ApplicationCommandOptionTypeBool, "value": value}
}

// The entity option types carry the ID as a string.

func User(name string, id snowflake.ID) Option {
	return Option{"name": name, "type": discord.ApplicationCommandOptionTypeUser, "value": id.String()}
}

func Channel(name string, id snowflake.ID) Option {
	return Option{"name": name, "type": discord.ApplicationCommandOptionTypeChannel, "value": id.String()}
}

func Role(name string, id snowflake.ID) Option {
	return Option{"name": name, "type": discord.ApplicationCommandOptionTypeRole, "value": id.String()}
}

// Discord sends what's typed as a string, whatever the option's type.
func Focused(o Option) Option {
	out := maps.Clone(o)
	out["focused"] = true
	out["value"] = fmt.Sprint(o["value"])
	return out
}

func options(opts []Option) []Option {
	if opts == nil {
		return []Option{}
	}
	return opts
}

// Builders start in GuildID and ChannelID, from a member without
// permissions.
type Interaction struct {
	payload map[string]any
}

func newInteraction(t discord.InteractionType, userID snowflake.ID, data map[string]any) *Interaction {
	i := &Interaction{payload: map[string]any{
		"id":             "1",
		"application_id": AppID.String(),
		"type":           t,
		"token":          "token",
		"version":        1,
		"locale":         "en-US",
		"data":           data,
	}}
	return i.InGuild(GuildID, ChannelID).asMember(userID, 0)
}

func (i *Interaction) asMember(userID snowflake.ID, perms discord.Permissions) *Interaction {
	i.payload["member"] = map[string]any{
		"user":        userPayload(userID),
		"roles":       []string{},
		"permissions": perms,
		"joined_at":   Timestamp,
	}
	delete(i.payload, "user")
	return i
}

func userPayload(id snowflake.ID) map[string]any {
	return map[string]any{"id": id.String(), "username": "user" + id.String()}
}

func (i *Interaction) InGuild(guildID, channelID snowflake.ID) *Interaction {
	i.payload["guild_id"] = guildID.String()
	i.payload["channel_id"] = channelID.String()
	i.payload["channel"] = map[string]any{
		"id": channelID.String(), "type": discord.ChannelTypeGuildText, "guild_id": guildID.String(), "name": "channel",
	}
	i.payload["context"] = discord.InteractionContextTypeGuild
	return i
}

func (i *Interaction) InDM(channelID snowflake.ID) *Interaction {
	member, _ := i.payload["member"].(map[string]any)
	if member != nil {
		i.payload["user"] = member["user"]
	}
	delete(i.payload, "member")
	delete(i.payload, "guild_id")
	delete(i.payload, "app_permissions")
	i.payload["channel_id"] = channelID.String()
	i.payload["channel"] = map[string]any{"id": channelID.String(), "type": discord.ChannelTypeDM}
	i.payload["context"] = discord.InteractionContextTypeBotDM
	return i
}

func (i *Interaction) WithPermissions(perms discord.Permissions) *Interaction {
	if member, ok := i.payload["member"].(map[string]any); ok {
		member["permissions"] = perms
	}
	return i
}

func (i *Interaction) WithAppPermissions(perms discord.Permissions) *Interaction {
	i.payload["app_permissions"] = perms
	return i
}

func (i *Interaction) WithRoles(roleIDs ...snowflake.ID) *Interaction {
	if member, ok := i.payload["member"].(map[string]any); ok {
		ids := make([]string, len(roleIDs))
		for n, id := range roleIDs {
			ids[n] = id.String()
		}
		member["roles"] = ids
	}
	return i
}

// kind is a field of Discord's resolved object, like "users" or "roles".
func (i *Interaction) WithResolved(kind string, entities map[string]any) *Interaction {
	data := i.payload["data"].(map[string]any)
	resolved, _ := data["resolved"].(map[string]any)
	if resolved == nil {
		resolved = map[string]any{}
		data["resolved"] = resolved
	}
	resolved[kind] = entities
	return i
}

// For changes the builders don't cover.
func (i *Interaction) Payload() map[string]any { return i.payload }

func (i *Interaction) Parse() discord.Interaction {
	raw, err := json.Marshal(i.payload)
	if err != nil {
		panic(err)
	}
	parsed, err := discord.UnmarshalInteraction(raw)
	if err != nil {
		panic(fmt.Sprintf("gumitest: %v: %s", err, raw))
	}
	return parsed
}

// Responses to the event go through c's REST client.
func (i *Interaction) Event(c *bot.Client) *events.InteractionCreate {
	parsed := i.Parse()
	return &events.InteractionCreate{
		GenericEvent: events.NewGenericEvent(c, 0, 0),
		Interaction:  parsed,
		Respond: func(t discord.InteractionResponseType, data discord.InteractionResponseData, opts ...rest.RequestOpt) error {
			return c.Rest.CreateInteractionResponse(parsed.ID(), parsed.Token(),
				discord.InteractionResponse{Type: t, Data: data}, opts...)
		},
	}
}

func Command(userID snowflake.ID, name string, opts ...Option) *Interaction {
	return newInteraction(discord.InteractionTypeApplicationCommand, userID, map[string]any{
		"id": "1", "name": name, "type": discord.ApplicationCommandTypeSlash, "options": options(opts),
	})
}

// Mark the option being typed with Focused.
func Autocomplete(userID snowflake.ID, name string, opts ...Option) *Interaction {
	return newInteraction(discord.InteractionTypeAutocomplete, userID, map[string]any{
		"id": "1", "name": name, "type": discord.ApplicationCommandTypeSlash, "options": options(opts),
	})
}

// The target message is in the interaction's channel, written by authorID.
func MessageCommand(userID snowflake.ID, name string, messageID, authorID snowflake.ID) *Interaction {
	i := newInteraction(discord.InteractionTypeApplicationCommand, userID, map[string]any{
		"id": "1", "name": name, "type": discord.ApplicationCommandTypeMessage, "target_id": messageID.String(),
	})
	return i.WithResolved("messages", map[string]any{messageID.String(): map[string]any{
		"id": messageID.String(), "channel_id": i.payload["channel_id"], "author": userPayload(authorID),
		"content": "", "timestamp": Timestamp,
	}})
}

func UserCommand(userID snowflake.ID, name string, targetID snowflake.ID) *Interaction {
	i := newInteraction(discord.InteractionTypeApplicationCommand, userID, map[string]any{
		"id": "1", "name": name, "type": discord.ApplicationCommandTypeUser, "target_id": targetID.String(),
	})
	i.WithResolved("users", map[string]any{targetID.String(): userPayload(targetID)})
	return i.WithResolved("members", map[string]any{targetID.String(): map[string]any{"roles": []string{}, "joined_at": Timestamp}})
}

func componentMessage() map[string]any {
	return map[string]any{
		"id": "4000", "channel_id": ChannelID.String(), "author": userPayload(BotID), "content": "", "timestamp": Timestamp,
	}
}

func Button(userID snowflake.ID, customID string) *Interaction {
	i := newInteraction(discord.InteractionTypeComponent, userID, map[string]any{
		"custom_id": customID, "component_type": discord.ComponentTypeButton,
	})
	i.payload["message"] = componentMessage()
	return i
}

func Select(userID snowflake.ID, customID string, values ...string) *Interaction {
	return SelectOf(discord.ComponentTypeStringSelectMenu, userID, customID, values...)
}

// Entity selects take IDs as values.
func SelectOf(kind discord.ComponentType, userID snowflake.ID, customID string, values ...string) *Interaction {
	if values == nil {
		values = []string{}
	}
	i := newInteraction(discord.InteractionTypeComponent, userID, map[string]any{
		"custom_id": customID, "component_type": kind, "values": values, "resolved": map[string]any{},
	})
	i.payload["message"] = componentMessage()
	return i
}

// inputs maps custom IDs to values. Each text input sits in a label.
func ModalSubmit(userID snowflake.ID, customID string, inputs map[string]string) *Interaction {
	labels := make([]map[string]any, 0, len(inputs))
	for id, value := range inputs {
		labels = append(labels, map[string]any{
			"type": discord.ComponentTypeLabel,
			"component": map[string]any{
				"type": discord.ComponentTypeTextInput, "custom_id": id, "value": value,
			},
		})
	}
	return newInteraction(discord.InteractionTypeModalSubmit, userID, map[string]any{
		"custom_id": customID, "components": labels,
	})
}

// guildID 0 makes it a DM.
func Message(c *bot.Client, guildID, channelID, authorID snowflake.ID, content string) *events.MessageCreate {
	m := discord.Message{
		ID: 5000, ChannelID: channelID, Content: content,
		Author: discord.User{ID: authorID, Username: "user" + authorID.String()},
	}
	var gid *snowflake.ID
	if guildID != 0 {
		gid = &guildID
		m.GuildID = gid
		m.Member = &discord.Member{}
	}
	return &events.MessageCreate{GenericMessage: &events.GenericMessage{
		GenericEvent: events.NewGenericEvent(c, 0, 0),
		MessageID:    m.ID, Message: m, ChannelID: channelID, GuildID: gid,
	}}
}

func GuildChannel(payload map[string]any) discord.GuildChannel {
	raw, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}
	var ch discord.UnmarshalChannel
	if err := json.Unmarshal(raw, &ch); err != nil {
		panic(fmt.Sprintf("gumitest: %v: %s", err, raw))
	}
	return ch.Channel.(discord.GuildChannel)
}
