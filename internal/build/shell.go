package build

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

const outputLimit = 4 << 10

type ExecRunner struct {
	Verbose bool
	Out     io.Writer
	Color   bool
}

func (r ExecRunner) Run(command Command) error {
	if r.Verbose {
		fmt.Fprintf(os.Stderr, "$ %s %s\n", command.Name, strings.Join(command.Args, " "))
	}
	var output strings.Builder
	sink := r.Out
	if sink == nil {
		sink = os.Stderr
	}
	cmd := exec.Command(command.Name, command.Args...)
	cmd.Dir = command.Dir
	cmd.Env = append(os.Environ(), command.Env...)
	if r.Color {
		cmd.Env = append(cmd.Env, "FORCE_COLOR=1", "npm_config_color=always")
	}
	cmd.Stdout = io.MultiWriter(sink, &output)
	cmd.Stderr = cmd.Stdout
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s: %w%s", command.Name, strings.Join(command.Args, " "), err,
			said(output.String()))
	}
	return nil
}

func said(output string) string {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return ""
	}
	if len(trimmed) > outputLimit {
		trimmed = "…" + trimmed[len(trimmed)-outputLimit:]
	}
	return "\n" + trimmed
}
