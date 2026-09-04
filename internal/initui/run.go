package initui

import (
	"errors"
	"io"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/apptivitypl/gopage/internal/scaffold"
)

var ErrCancelled = errors.New("cancelled")

func Run(config scaffold.Config, in io.Reader, out io.Writer) (scaffold.Config, error) {
	program := tea.NewProgram(New(config), tea.WithInput(in), tea.WithOutput(out))
	final, err := program.Run()
	if err != nil {
		return scaffold.Config{}, err
	}
	model, ok := final.(Model)
	if !ok || !model.Accepted() {
		return scaffold.Config{}, ErrCancelled
	}
	return model.Config(), nil
}
