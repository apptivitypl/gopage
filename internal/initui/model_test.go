package initui

import (
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sonquer/rill/internal/scaffold"
)

func press(model Model, keys ...tea.KeyMsg) Model {
	for _, key := range keys {
		next, _ := model.Update(key)
		model = next.(Model)
	}
	return model
}

func typing(text string) []tea.KeyMsg {
	keys := make([]tea.KeyMsg, 0, len(text))
	for _, r := range text {
		if r == ' ' {
			keys = append(keys, tea.KeyMsg{Type: tea.KeySpace})
			continue
		}
		keys = append(keys, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	return keys
}

var (
	enter = tea.KeyMsg{Type: tea.KeyEnter}
	down  = tea.KeyMsg{Type: tea.KeyDown}
	up    = tea.KeyMsg{Type: tea.KeyUp}
	esc   = tea.KeyMsg{Type: tea.KeyEsc}
)

func start(t *testing.T) Model {
	t.Helper()
	return New(scaffold.Config{Dir: "demo", Name: "demo"})
}

func TestTheConfiguratorWalksEveryQuestion(t *testing.T) {
	model := start(t)
	steps := []Step{StepModule, StepName, StepTemplate, StepLocales, StepNav, StepCSS, StepTheme, StepReact}
	for _, want := range steps {
		if model.Step() != want {
			t.Fatalf("step = %v, want %v", model.Step(), want)
		}
		if model.Step() == StepModule {
			model = press(model, typing("example.com/demo")...)
		}
		model = press(model, enter)
	}
	if model.Step() != StepDone || !model.Accepted() {
		t.Fatalf("step = %v, accepted = %v", model.Step(), model.Accepted())
	}
}

func TestTheAnswersReachTheConfig(t *testing.T) {
	model := start(t)
	model = press(model, typing("example.com/demo")...)
	model = press(model, enter)
	for range len("demo") {
		model = press(model, tea.KeyMsg{Type: tea.KeyBackspace})
	}
	model = press(model, typing("my site")...)
	model = press(model, enter)
	model = press(model, enter)
	model = press(model, typing("en, pl, de")...)
	model = press(model, enter)
	model = press(model, down, enter)
	model = press(model, enter)

	config := model.Config()
	if config.Module != "example.com/demo" || config.Name != "my site" {
		t.Errorf("config = %+v", config)
	}
	if !slices.Equal(config.Locales, []string{"en", "pl", "de"}) || config.DefaultLocale != "en" {
		t.Errorf("locales = %v, default = %q", config.Locales, config.DefaultLocale)
	}
	if config.Nav != scaffold.NavOff {
		t.Errorf("nav = %q, want the second option", config.Nav)
	}
	if config.CSS != scaffold.CSSPlain {
		t.Errorf("css = %q", config.CSS)
	}
}

func TestTheModuleQuestionInsists(t *testing.T) {
	model := press(start(t), enter)
	if model.Step() != StepModule {
		t.Errorf("step = %v, want the module question again", model.Step())
	}
}

func TestABlankNameKeepsTheDefault(t *testing.T) {
	model := press(start(t), typing("example.com/demo")...)
	model = press(model, enter)
	for range len("demo") {
		model = press(model, tea.KeyMsg{Type: tea.KeyBackspace})
	}
	model = press(model, enter)
	if model.Config().Name != "demo" {
		t.Errorf("name = %q, want the directory kept", model.Config().Name)
	}
}

func TestChoicesWrapAround(t *testing.T) {
	model := press(start(t), typing("example.com/demo")...)
	model = press(model, enter, enter)
	if model.Step() != StepTemplate {
		t.Fatalf("step = %v", model.Step())
	}
	first := model.current().Answer[0]
	model = press(model, up)
	last := model.current().Answer[model.cursor]
	if len(model.current().Answer) > 1 && first == last {
		t.Errorf("up from the first option must wrap to the last, got %q", last)
	}
	model = press(model, down)
	if model.current().Answer[model.cursor] != first {
		t.Errorf("cursor = %d, want back at the first option", model.cursor)
	}
}

func TestTypingIsIgnoredOnAChoice(t *testing.T) {
	model := press(start(t), typing("example.com/demo")...)
	model = press(model, enter, enter)
	before := model.input
	model = press(model, typing("noise")...)
	model = press(model, tea.KeyMsg{Type: tea.KeyBackspace}, up, down)
	if model.input != before {
		t.Errorf("input = %q, want a choice to ignore typing", model.input)
	}
}

func TestEscapeStops(t *testing.T) {
	model := press(start(t), esc)
	if !model.Quit() || model.Accepted() {
		t.Errorf("quit = %v, accepted = %v", model.Quit(), model.Accepted())
	}
}

func TestUnknownMessagesChangeNothing(t *testing.T) {
	model := start(t)
	next, cmd := model.Update(tea.WindowSizeMsg{Width: 10})
	if cmd != nil || next.(Model).Step() != StepModule {
		t.Error("an unrelated message must leave the model alone")
	}
	if model.Init() != nil {
		t.Error("the model starts without a command")
	}
}

func TestEmptyLocalesFallBack(t *testing.T) {
	model := press(start(t), typing("example.com/demo")...)
	model = press(model, enter, enter, enter)
	model = press(model, enter)
	if !slices.Equal(model.Config().Locales, []string{"en"}) {
		t.Errorf("locales = %v", model.Config().Locales)
	}
}

func TestTheViewShowsTheQuestionAndTheAnswers(t *testing.T) {
	model := start(t)
	view := model.View()
	if !strings.Contains(view, "go module path") || !strings.Contains(view, "rill new demo") {
		t.Errorf("view = %q", view)
	}
	model = press(model, typing("example.com/demo")...)
	model = press(model, enter)
	view = model.View()
	if !strings.Contains(view, "example.com/demo") || !strings.Contains(view, "project name") {
		t.Errorf("view = %q", view)
	}
	model = press(model, enter, enter, enter, enter, enter, enter, enter)
	if !strings.Contains(model.View(), "writing the project") {
		t.Errorf("view = %q", model.View())
	}
}

func TestTheViewListsChoices(t *testing.T) {
	model := press(start(t), typing("example.com/demo")...)
	model = press(model, enter, enter)
	view := model.View()
	for _, want := range scaffold.Names() {
		if !strings.Contains(view, want) {
			t.Errorf("view = %q, want %q", view, want)
		}
	}
	if !strings.Contains(view, cursorMark) {
		t.Errorf("view = %q, want a cursor", view)
	}
}

func TestKeysWithoutAMeaningAreIgnored(t *testing.T) {
	model := press(start(t), tea.KeyMsg{Type: tea.KeyTab}, tea.KeyMsg{Type: tea.KeyHome})
	if model.Step() != StepModule || model.input != "" {
		t.Errorf("step = %v, input = %q", model.Step(), model.input)
	}
}

func TestArrowsDoNothingWhileTyping(t *testing.T) {
	model := press(start(t), typing("example.com")...)
	before := model.input
	model = press(model, up, down)
	if model.input != before || model.cursor != 0 {
		t.Errorf("input = %q, cursor = %d", model.input, model.cursor)
	}
}

func TestBackspaceOnAnEmptyFieldIsHarmless(t *testing.T) {
	model := press(start(t), tea.KeyMsg{Type: tea.KeyBackspace})
	if model.input != "" {
		t.Errorf("input = %q", model.input)
	}
}

func TestKeysAfterTheLastQuestionAreIgnored(t *testing.T) {
	model := press(start(t), typing("example.com/demo")...)
	model = press(model, enter, enter, enter, enter, enter, enter, enter, enter)
	if model.Step() != StepDone || !model.Accepted() {
		t.Fatalf("step = %v, accepted = %v", model.Step(), model.Accepted())
	}
	after := press(model, enter, down, typing("noise")[0])
	if !after.Accepted() || after.Config().Module != "example.com/demo" {
		t.Errorf("config = %+v", after.Config())
	}
}
