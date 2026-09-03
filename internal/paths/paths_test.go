package paths

import (
	"path"
	"strings"
	"testing"
)

func TestGeneratedListsEveryMachineOwnedRoot(t *testing.T) {
	got := Generated()
	for _, want := range []string{GenRoot, DistRoot, CacheRoot} {
		if !contains(got, want) {
			t.Errorf("Generated() = %v, missing %q", got, want)
		}
	}
}

func TestGeneratedIsNotSharedBetweenCallers(t *testing.T) {
	first := Generated()
	first[0] = "clobbered"
	if Generated()[0] == "clobbered" {
		t.Error("Generated must hand back a fresh slice")
	}
}

func TestTheGeneratedGoTreeIsImportable(t *testing.T) {
	for _, part := range strings.Split(GenRoot, "/") {
		if strings.HasPrefix(part, ".") || strings.HasPrefix(part, "_") {
			t.Fatalf("GenRoot = %q, which the go tool ignores", GenRoot)
		}
	}
}

func TestEmbeddedTreesSitInsideTheGeneratedPackage(t *testing.T) {
	for _, name := range []string{Manifest, GenConfig, GenStyles, GenPublic, GenBundles, PropsDir, PropsTypes} {
		if path.Dir(name) != GenRoot {
			t.Errorf("%q must sit directly in %q so go:embed can reach it", name, GenRoot)
		}
	}
}

func TestTheEmbedMountsMatchTheSourceDirectories(t *testing.T) {
	if path.Base(GenStyles) != StylesDir {
		t.Errorf("%q and %q must agree: one names the source tree, the other the embed mount", GenStyles, StylesDir)
	}
	if path.Base(GenPublic) != PublicDir {
		t.Errorf("%q and %q must agree", GenPublic, PublicDir)
	}
}

func TestTheCacheIsHiddenFromTheGoTool(t *testing.T) {
	if !strings.HasPrefix(CacheRoot, ".") {
		t.Errorf("CacheRoot = %q, want a dot-directory the go tool skips", CacheRoot)
	}
	if !strings.HasPrefix(Inventory, CacheDir+"/") || !strings.HasPrefix(CacheDir, CacheRoot+"/") {
		t.Errorf("the cache paths do not nest: %q, %q, %q", CacheRoot, CacheDir, Inventory)
	}
}

func TestDeployableOutputSitsUnderOneRoot(t *testing.T) {
	for _, name := range []string{AssetsDir, Redirects, Headers, WorkerDir, WorkerBinary, NativeBinary} {
		if !strings.HasPrefix(name, DistRoot+"/") {
			t.Errorf("%q must sit under %q", name, DistRoot)
		}
	}
}

func TestEntryPointsAreRelativePackagePaths(t *testing.T) {
	for _, name := range []string{ServerMain, WorkerMain} {
		if !strings.HasPrefix(name, "./") {
			t.Errorf("%q must be a relative package path", name)
		}
	}
	if path.Base(WorkerMain) != "worker" || path.Base(ServerMain) != "server" {
		t.Errorf("entry points = %q, %q", ServerMain, WorkerMain)
	}
}

func contains(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}
