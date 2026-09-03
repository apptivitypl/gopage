package initui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/apptivitypl/rill/internal/scaffold"
)

var (
	titleStyle    = lipgloss.NewStyle().Bold(true)
	helpStyle     = lipgloss.NewStyle().Faint(true)
	selectedStyle = lipgloss.NewStyle().Bold(true)
	answerStyle   = lipgloss.NewStyle().Faint(true)
	cursorMark    = "> "
	blankMark     = "  "
)

func (m Model) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("rill new " + m.config.Dir))
	b.WriteString("\n\n")
	m.writeAnswered(&b)
	if m.Step() == StepDone {
		b.WriteString(helpStyle.Render("writing the project"))
		b.WriteString("\n")
		return b.String()
	}
	m.writeQuestion(&b)
	return b.String()
}

func (m Model) writeAnswered(b *strings.Builder) {
	for i := range m.step {
		step := order[i]
		fmt.Fprintf(b, "%s%s %s\n", blankMark, questions[step].Title+":", answerStyle.Render(answerFor(m.config, step)))
	}
	if m.step > 0 {
		b.WriteString("\n")
	}
}

func (m Model) writeQuestion(b *strings.Builder) {
	current := m.current()
	b.WriteString(titleStyle.Render(current.Title))
	b.WriteString("\n")
	if current.Help != "" {
		b.WriteString(helpStyle.Render(current.Help))
		b.WriteString("\n")
	}
	if current.Text {
		fmt.Fprintf(b, "\n%s%s\n", cursorMark, m.input)
		b.WriteString("\n")
		b.WriteString(helpStyle.Render("enter to continue, esc to stop"))
		b.WriteString("\n")
		return
	}
	b.WriteString("\n")
	for index, option := range current.Answer {
		mark := blankMark
		text := option
		if index == m.cursor {
			mark = cursorMark
			text = selectedStyle.Render(option)
		}
		fmt.Fprintf(b, "%s%s\n", mark, text)
	}
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("up and down to choose, enter to continue, esc to stop"))
	b.WriteString("\n")
}

func answerFor(config scaffold.Config, step Step) string {
	switch step {
	case StepModule:
		return config.Module
	case StepName:
		return config.Name
	case StepTemplate:
		return config.Template
	case StepLocales:
		return strings.Join(config.Locales, ", ")
	case StepNav:
		return config.Nav
	case StepCSS:
		return config.CSS
	default:
		return ""
	}
}
