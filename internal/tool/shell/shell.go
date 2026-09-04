package shell

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

func Run(name string, args ...string) (string, error) {
	fmt.Fprintf(os.Stderr, "$ %s %s\n", name, strings.Join(args, " "))
	var output strings.Builder
	cmd := exec.Command(name, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = io.MultiWriter(os.Stdout, &output)
	cmd.Stderr = io.MultiWriter(os.Stderr, &output)
	if err := cmd.Run(); err != nil {
		return output.String(), fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return output.String(), nil
}

func Capture(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).Output()
	if err != nil {
		return "", fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return string(out), nil
}
