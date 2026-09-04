package initui

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"

	"github.com/apptivitypl/gopage/internal/scaffold"
)

func TestTheConfiguratorRunsOnATerminal(t *testing.T) {
	model := New(scaffold.Config{Dir: "demo", Name: "demo"})
	test := teatest.NewTestModel(t, model, teatest.WithInitialTermSize(80, 24))

	teatest.WaitFor(t, test.Output(), func(out []byte) bool {
		return bytes.Contains(out, []byte("go module path"))
	}, teatest.WithDuration(3*time.Second))

	test.Type("example.com/demo")
	for range len(order) {
		test.Send(tea.KeyMsg{Type: tea.KeyEnter})
	}

	final := test.FinalModel(t, teatest.WithFinalTimeout(3*time.Second)).(Model)
	if !final.Accepted() {
		t.Fatalf("the configurator did not finish: %+v", final.Config())
	}
	if final.Config().Module != "example.com/demo" {
		t.Errorf("config = %+v", final.Config())
	}
}

func TestRunReadsAnAnsweredSession(t *testing.T) {
	in := bytes.NewBufferString("example.com/demo\r\r\r\r\r\r\r\r")
	var out bytes.Buffer
	done := make(chan struct{})
	var config scaffold.Config
	var err error
	go func() {
		defer close(done)
		config, err = Run(scaffold.Config{Dir: "demo", Name: "demo"}, in, &out)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the configurator never finished")
	}
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if config.Module != "example.com/demo" {
		t.Errorf("config = %+v", config)
	}
}

func TestRunReportsACancelledSession(t *testing.T) {
	in := bytes.NewBufferString("\x1b")
	var out bytes.Buffer
	done := make(chan struct{})
	var err error
	go func() {
		defer close(done)
		_, err = Run(scaffold.Config{Dir: "demo"}, in, &out)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the configurator never finished")
	}
	if !errors.Is(err, ErrCancelled) {
		t.Errorf("err = %v, want ErrCancelled", err)
	}
}

func TestAnAnsweredSessionAndFlagsAgree(t *testing.T) {
	answers := "example.com/demo\r\r\rpl, en\r\r\r\r\r"
	in := bytes.NewBufferString(answers)
	var out bytes.Buffer
	done := make(chan struct{})
	var fromUI scaffold.Config
	var err error
	go func() {
		defer close(done)
		fromUI, err = Run(scaffold.Config{Dir: "demo", Name: "demo"}, in, &out)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the configurator never finished")
	}
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	fromFlags := scaffold.Config{
		Dir:           "demo",
		Name:          "demo",
		Module:        "example.com/demo",
		Template:      fromUI.Template,
		Locales:       []string{"pl", "en"},
		DefaultLocale: "pl",
		Nav:           fromUI.Nav,
		CSS:           fromUI.CSS,
		Theme:         fromUI.Theme,
		React:         fromUI.React,
	}
	if !reflect.DeepEqual(fromUI, fromFlags) {
		t.Errorf("the configurator produced %+v, the flags produce %+v", fromUI, fromFlags)
	}

	uiDir := t.TempDir() + "/ui"
	flagDir := t.TempDir() + "/flags"
	fromUI.Dir = uiDir
	fromFlags.Dir = flagDir
	if err := scaffold.Create(fromUI); err != nil {
		t.Fatalf("create from the configurator: %v", err)
	}
	if err := scaffold.Create(fromFlags); err != nil {
		t.Fatalf("create from the flags: %v", err)
	}
	if !reflect.DeepEqual(files(t, uiDir), files(t, flagDir)) {
		t.Error("the configurator and the flags produced different projects")
	}
}

func files(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		relative, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out[filepath.ToSlash(relative)] = string(data)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
	return out
}
