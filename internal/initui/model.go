package initui

import (
	"github.com/apptivitypl/gopage/internal/paths"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/apptivitypl/gopage/internal/scaffold"
)

type Step uint8

const (
	StepModule Step = iota
	StepName
	StepTemplate
	StepLocales
	StepNav
	StepCSS
	StepTheme
	StepReact
	StepDone
)

var order = []Step{StepModule, StepName, StepTemplate, StepLocales, StepNav, StepCSS, StepTheme, StepReact}

type question struct {
	Title  string
	Help   string
	Text   bool
	Answer []string
}

var questions = map[Step]question{
	StepModule:   {Title: "go module path", Help: "for example example.com/myapp", Text: true},
	StepName:     {Title: "project name", Help: "shown in " + paths.Config + ", defaults to the directory", Text: true},
	StepTemplate: {Title: "starting template", Answer: scaffold.Names()},
	StepLocales:  {Title: "languages", Help: "comma separated, the first one is the default", Text: true},
	StepNav:      {Title: "navigation", Answer: []string{scaffold.NavPartial, scaffold.NavOff}},
	StepCSS:      {Title: "css", Answer: []string{scaffold.CSSPlain, scaffold.CSSTailwind}},
	StepTheme:    {Title: "theme", Help: "toggle adds a light/dark switch to the header", Answer: scaffold.Themes()},
	StepReact:    {Title: "react", Help: "react components in the browser, preact/compat, or none", Answer: scaffold.Reacts()},
}

type Model struct {
	config   scaffold.Config
	step     int
	cursor   int
	input    string
	quit     bool
	accepted bool
}

func New(config scaffold.Config) Model {
	model := Model{config: config}
	model.input = model.prefill()
	return model
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Step() Step {
	if m.step >= len(order) {
		return StepDone
	}
	return order[m.step]
}

func (m Model) Config() scaffold.Config {
	return m.config
}

func (m Model) Accepted() bool {
	return m.accepted
}

func (m Model) Quit() bool {
	return m.quit
}

func (m Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := message.(tea.KeyMsg)
	if !ok || m.Step() == StepDone {
		return m, nil
	}
	switch key.Type {
	case tea.KeyCtrlC, tea.KeyEsc:
		m.quit = true
		return m, tea.Quit
	case tea.KeyEnter:
		return m.commit()
	case tea.KeyUp:
		return m.move(-1), nil
	case tea.KeyDown:
		return m.move(1), nil
	case tea.KeyBackspace:
		return m.erase(), nil
	case tea.KeyRunes, tea.KeySpace:
		return m.typed(key), nil
	default:
		return m, nil
	}
}

func (m Model) current() question {
	return questions[m.Step()]
}

func (m Model) move(delta int) Model {
	if m.current().Text {
		return m
	}
	options := len(m.current().Answer)
	m.cursor = (m.cursor + delta + options) % options
	return m
}

func (m Model) erase() Model {
	if !m.current().Text || m.input == "" {
		return m
	}
	m.input = m.input[:len(m.input)-1]
	return m
}

func (m Model) typed(key tea.KeyMsg) Model {
	if !m.current().Text {
		return m
	}
	if key.Type == tea.KeySpace {
		m.input += " "
		return m
	}
	m.input += string(key.Runes)
	return m
}

func (m Model) commit() (tea.Model, tea.Cmd) {
	answer := m.input
	if !m.current().Text {
		answer = m.current().Answer[m.cursor]
	}
	if m.current().Text && strings.TrimSpace(answer) == "" && m.Step() == StepModule {
		return m, nil
	}
	m.config = apply(m.config, m.Step(), answer)
	m.step++
	m.cursor = 0
	if m.Step() == StepDone {
		m.accepted = true
		return m, tea.Quit
	}
	m.input = m.prefill()
	return m, nil
}

func (m Model) prefill() string {
	switch m.Step() {
	case StepModule:
		return m.config.Module
	case StepName:
		return m.config.Name
	case StepLocales:
		return strings.Join(m.config.Locales, ", ")
	default:
		return ""
	}
}

func apply(config scaffold.Config, step Step, answer string) scaffold.Config {
	answer = strings.TrimSpace(answer)
	switch step {
	case StepModule:
		config.Module = answer
	case StepName:
		if answer != "" {
			config.Name = answer
		}
	case StepTemplate:
		config.Template = answer
	case StepLocales:
		config.Locales, config.DefaultLocale = parseLocales(answer, config.DefaultLocale)
	case StepNav:
		config.Nav = answer
	case StepCSS:
		config.CSS = answer
	case StepTheme:
		config.Theme = answer
	case StepReact:
		config.React = answer
	case StepDone:
	}
	return config
}

func parseLocales(answer, fallback string) ([]string, string) {
	var locales []string
	for part := range strings.SplitSeq(answer, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			locales = append(locales, trimmed)
		}
	}
	if len(locales) == 0 {
		if fallback == "" {
			fallback = "en"
		}
		return []string{fallback}, fallback
	}
	return locales, locales[0]
}
