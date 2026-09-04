package build

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func answers(gowork string, use []string, fail bool) asker {
	return func(dir, name string, args ...string) ([]byte, error) {
		if fail {
			return nil, errors.New("no go on this machine")
		}
		if len(args) >= 2 && args[0] == "env" && args[1] == "GOWORK" {
			return []byte(gowork + "\n"), nil
		}
		body := `{"Use":[`
		for i, path := range use {
			if i > 0 {
				body += ","
			}
			body += `{"DiskPath":"` + path + `"}`
		}
		return []byte(body + "]}"), nil
	}
}

func TestNoWorkspaceLeavesTheEnvironmentAlone(t *testing.T) {
	if got := outsideWorkspace(t.TempDir(), "go", answers("", nil, false)); got != nil {
		t.Errorf("env = %v, want nothing when no workspace is active", got)
	}
}

func TestAWorkspaceThatListsTheProjectIsOurs(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "examples", "blog")
	if got := outsideWorkspace(project, "go", answers(filepath.Join(root, "go.work"), []string{"./examples/blog"}, false)); got != nil {
		t.Errorf("env = %v, want the workspace left active", got)
	}
}

func TestAWorkspaceWithoutTheProjectIsIgnored(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "demo", "my-site")
	got := outsideWorkspace(project, "go", answers(filepath.Join(root, "go.work"), []string{".", "./examples/blog"}, false))
	if !slices.Equal(got, []string{workspaceOff}) {
		t.Errorf("env = %v, want the workspace switched off", got)
	}
}

func TestAnAbsoluteMemberStillMatches(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "site")
	if got := outsideWorkspace(project, "go", answers(filepath.Join(root, "go.work"), []string{project}, false)); got != nil {
		t.Errorf("env = %v, want an absolute use directive to count", got)
	}
}

func TestAToolchainThatCannotAnswerChangesNothing(t *testing.T) {
	if got := outsideWorkspace(t.TempDir(), "go", answers("", nil, true)); got != nil {
		t.Errorf("env = %v, want nothing when go cannot be asked", got)
	}
}

func TestUnreadableWorkspaceJSONChangesNothing(t *testing.T) {
	root := t.TempDir()
	broken := func(dir, name string, args ...string) ([]byte, error) {
		if len(args) >= 2 && args[0] == "env" {
			return []byte(filepath.Join(root, "go.work")), nil
		}
		return []byte("not json"), nil
	}
	if got := outsideWorkspace(filepath.Join(root, "site"), "go", broken); got != nil {
		t.Errorf("env = %v, want nothing when the workspace cannot be read", got)
	}
}

func TestTheEnclosingModuleIsFound(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/held\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if got := EnclosingModule(nested); got != resolved(root) {
		t.Errorf("EnclosingModule = %q, want %q", got, resolved(root))
	}
}

func TestAModuleOfItsOwnDoesNotCount(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "site")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(project, "go.mod"), []byte("module example.com/site\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if got := EnclosingModule(project); got != "" {
		t.Errorf("EnclosingModule = %q, want nothing when only the project itself has a go.mod", got)
	}
}

func TestADirectoryThatDoesNotExistYetLooksUpwards(t *testing.T) {
	root := t.TempDir()
	if got := nearest(filepath.Join(root, "not", "written", "yet")); got != resolved(root) {
		t.Errorf("nearest = %q, want the closest directory that exists", got)
	}
}

func TestAskingTheRealToolchainOutsideAWorkspaceChangesNothing(t *testing.T) {
	if got := OutsideWorkspace(t.TempDir(), "go"); got != nil {
		t.Errorf("env = %v, want nothing outside any workspace", got)
	}
}

func TestAWorkspaceThatWillNotDescribeItselfChangesNothing(t *testing.T) {
	root := t.TempDir()
	silent := func(dir, name string, args ...string) ([]byte, error) {
		if len(args) >= 2 && args[0] == "env" {
			return []byte(filepath.Join(root, "go.work")), nil
		}
		return nil, errors.New("go work edit refused")
	}
	if got := outsideWorkspace(filepath.Join(root, "site"), "go", silent); got != nil {
		t.Errorf("env = %v, want nothing when the workspace will not describe itself", got)
	}
}

func TestResolvingWalksThroughASymlink(t *testing.T) {
	root := t.TempDir()
	real := filepath.Join(root, "real")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("this filesystem refuses symlinks: %v", err)
	}
	if resolved(link) != resolved(real) {
		t.Errorf("resolved(%q) = %q, want it to land on %q", link, resolved(link), resolved(real))
	}
}
