package gumi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/bwmarrin/discordgo"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
)

// recorder captures the requests a session sends and answers 204.
type recorder struct {
	mu     sync.Mutex
	bodies []map[string]any
}

func (rt *recorder) RoundTrip(req *http.Request) (*http.Response, error) {
	var body map[string]any
	if req.Body != nil {
		raw, _ := io.ReadAll(req.Body)
		_ = json.Unmarshal(raw, &body)
	}

	rt.mu.Lock()
	rt.bodies = append(rt.bodies, body)
	rt.mu.Unlock()

	return &http.Response{
		StatusCode: http.StatusNoContent,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader("")),
		Request:    req,
	}, nil
}

func (rt *recorder) sent() []map[string]any {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.bodies
}

var _ = ginkgo.Describe("Autocomplete", func() {
	var (
		r   *Router
		s   *discordgo.Session
		rec *recorder
	)

	ginkgo.BeforeEach(func() {
		rec = &recorder{}
		var err error
		s, err = discordgo.New("Bot test")
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		s.Client = &http.Client{Transport: rec}
		r = New(Config{})
	})

	typing := func(name string, opts ...*discordgo.ApplicationCommandInteractionDataOption) *discordgo.InteractionCreate {
		return &discordgo.InteractionCreate{Interaction: &discordgo.Interaction{
			ID: "1", Token: "t", GuildID: "g",
			Type:   discordgo.InteractionApplicationCommandAutocomplete,
			Member: &discordgo.Member{User: &discordgo.User{ID: "u"}},
			Data: discordgo.ApplicationCommandInteractionData{
				Name: name, CommandType: discordgo.ChatApplicationCommand, Options: opts,
			},
		}}
	}

	str := func(name, value string, focused bool) *discordgo.ApplicationCommandInteractionDataOption {
		return &discordgo.ApplicationCommandInteractionDataOption{
			Name: name, Type: OptionString, Value: value, Focused: focused,
		}
	}

	sub := func(name string, opts ...*discordgo.ApplicationCommandInteractionDataOption) *discordgo.ApplicationCommandInteractionDataOption {
		return &discordgo.ApplicationCommandInteractionDataOption{
			Name: name, Type: discordgo.ApplicationCommandOptionSubCommand, Options: opts,
		}
	}

	choicesSent := func() []any {
		sent := rec.sent()
		gomega.Expect(sent).To(gomega.HaveLen(1))
		gomega.Expect(sent[0]).To(gomega.HaveKeyWithValue("type", float64(discordgo.InteractionApplicationCommandAutocompleteResult)))
		data, ok := sent[0]["data"].(map[string]any)
		gomega.Expect(ok).To(gomega.BeTrue())
		gomega.Expect(data).To(gomega.HaveKey("choices"))
		choices, _ := data["choices"].([]any)
		return choices
	}

	ginkgo.It("marks the option as autocompleted in the definition", func() {
		r.MustRegister(&Command{Name: "relay", Description: "d", Handler: testOK, Options: []*Option{
			String("target", "d").Require().WithAutocomplete(func(*AutocompleteContext) ([]Choice, error) { return nil, nil }),
			String("note", "d"),
		}})

		global, _ := r.ApplicationCommands()
		gomega.Expect(global[0].Options[0].Autocomplete).To(gomega.BeTrue())
		gomega.Expect(global[0].Options[1].Autocomplete).To(gomega.BeFalse())
	})

	ginkgo.DescribeTable("rejects invalid declarations",
		func(o *Option, msg string) {
			err := r.Register(&Command{Name: "x", Description: "d", Handler: testOK, Options: []*Option{o}})
			gomega.Expect(err).To(gomega.MatchError(gomega.ContainSubstring(msg)))
		},
		ginkgo.Entry("on a boolean", Boolean("b", "d").WithAutocomplete(func(*AutocompleteContext) ([]Choice, error) { return nil, nil }),
			"only string, integer and number options can autocomplete"),
		ginkgo.Entry("with choices", String("s", "d").WithChoices(Choice{"a", "a"}).WithAutocomplete(func(*AutocompleteContext) ([]Choice, error) { return nil, nil }),
			"cannot be combined with choices"),
	)

	ginkgo.It("routes through subcommands to the focused option", func() {
		var got *AutocompleteContext
		r.MustRegister(&Command{Name: "relay", Description: "d", Subcommands: []*Command{
			{Name: "add", Description: "d", Handler: testOK, Options: []*Option{
				String("target", "d").Require().WithAutocomplete(func(ctx *AutocompleteContext) ([]Choice, error) {
					got = ctx
					return []Choice{{"Pomu Rainpuff", "UCP4nMSTdwU1KqYWu3UH5DHQ"}}, nil
				}),
				String("note", "d"),
			}},
		}})

		r.HandleInteraction(s, typing("relay", sub("add", str("note", "hi", false), str("target", "pom", true))))

		gomega.Expect(got).NotTo(gomega.BeNil())
		gomega.Expect(got.Command.QualifiedName()).To(gomega.Equal("relay add"))
		gomega.Expect(got.Option.Name).To(gomega.Equal("target"))
		gomega.Expect(got.Value).To(gomega.Equal("pom"))
		gomega.Expect(got.Options.String("note")).To(gomega.Equal("hi"))
		gomega.Expect(got.UserID()).To(gomega.Equal("u"))
		_, hasDeadline := got.Context().Deadline()
		gomega.Expect(hasDeadline).To(gomega.BeTrue())

		gomega.Expect(choicesSent()).To(gomega.Equal([]any{
			map[string]any{"name": "Pomu Rainpuff", "value": "UCP4nMSTdwU1KqYWu3UH5DHQ"},
		}))
	})

	ginkgo.It("fits choices to Discord's limits", func() {
		long := strings.Repeat("x", 150)
		r.MustRegister(&Command{Name: "pick", Description: "d", Handler: testOK, Options: []*Option{
			String("q", "d").WithAutocomplete(func(*AutocompleteContext) ([]Choice, error) {
				choices := []Choice{{long, "ok"}, {"bad value", long}, {"", "nameless"}}
				for range 30 {
					choices = append(choices, Choice{"n", "v"})
				}
				return choices, nil
			}),
		}})

		r.HandleInteraction(s, typing("pick", str("q", "", true)))

		choices := choicesSent()
		gomega.Expect(choices).To(gomega.HaveLen(maxEntries))
		first := choices[0].(map[string]any)
		gomega.Expect([]rune(first["name"].(string))).To(gomega.HaveLen(maxChoiceLength))
		gomega.Expect(first["name"]).To(gomega.HaveSuffix("…"))
		gomega.Expect(choices[1]).To(gomega.Equal(map[string]any{"name": "n", "value": "v"}))
	})

	ginkgo.It("answers with no choices when a check fails, without calling the handler", func() {
		called := false
		r.MustRegister(&Command{Name: "secret", Description: "d", Handler: testOK,
			Checks: []Check{func(*Context) error { return errors.New("nope") }},
			Options: []*Option{String("q", "d").WithAutocomplete(func(*AutocompleteContext) ([]Choice, error) {
				called = true
				return []Choice{{"a", "a"}}, nil
			})},
		})

		r.HandleInteraction(s, typing("secret", str("q", "a", true)))

		gomega.Expect(called).To(gomega.BeFalse())
		gomega.Expect(choicesSent()).To(gomega.BeEmpty())
	})

	ginkgo.It("reports handler errors and panics, answering with no choices", func() {
		var errs []error
		r = New(Config{AutocompleteErrorHandler: func(_ *AutocompleteContext, err error) { errs = append(errs, err) }})
		r.MustRegister(
			&Command{Name: "fails", Description: "d", Handler: testOK, Options: []*Option{
				String("q", "d").WithAutocomplete(func(*AutocompleteContext) ([]Choice, error) { return nil, errors.New("boom") }),
			}},
			&Command{Name: "panics", Description: "d", Handler: testOK, Options: []*Option{
				String("q", "d").WithAutocomplete(func(*AutocompleteContext) ([]Choice, error) { panic("oh no") }),
			}},
		)

		r.HandleInteraction(s, typing("fails", str("q", "a", true)))
		r.HandleInteraction(s, typing("panics", str("q", "a", true)))

		gomega.Expect(errs).To(gomega.HaveLen(2))
		gomega.Expect(errs[0]).To(gomega.MatchError("boom"))
		var pe *PanicError
		gomega.Expect(errors.As(errs[1], &pe)).To(gomega.BeTrue())
		gomega.Expect(pe.Value).To(gomega.Equal("oh no"))

		for _, body := range rec.sent() {
			gomega.Expect(body["data"]).To(gomega.Equal(map[string]any{"choices": []any{}}))
		}
	})

	ginkgo.It("ignores unknown commands and options without a handler", func() {
		r.MustRegister(&Command{Name: "plain", Description: "d", Handler: testOK, Options: []*Option{String("q", "d")}})

		r.HandleInteraction(s, typing("plain", str("q", "a", true)))
		r.HandleInteraction(s, typing("missing", str("q", "a", true)))

		gomega.Expect(rec.sent()).To(gomega.BeEmpty())
	})

	ginkgo.It("does nothing when slash commands are disabled", func() {
		r = New(Config{DisableSlashCommands: true})
		r.MustRegister(&Command{Name: "pick", Description: "d", Handler: testOK, Options: []*Option{
			String("q", "d").WithAutocomplete(func(*AutocompleteContext) ([]Choice, error) { return nil, nil }),
		}})

		r.HandleInteraction(s, typing("pick", str("q", "a", true)))

		gomega.Expect(rec.sent()).To(gomega.BeEmpty())
	})
})
