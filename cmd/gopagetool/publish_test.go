package main

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/apptivitypl/gopage/internal/tool/npmpkg"
)

type registry struct {
	held      map[string]bool
	published []string
	answers   map[string]error
	said      map[string]string
	slept     int
}

func (g *registry) job(t *testing.T) npmRelease {
	t.Helper()
	var log strings.Builder
	return npmRelease{
		version:  "0.2.0",
		assemble: func(member string) (string, error) { return "dist/npm/" + member, nil },
		present:  func(member string) (bool, error) { return g.held[member], nil },
		publish: func(dir string) (string, error) {
			member := strings.TrimPrefix(dir, "dist/npm/")
			g.published = append(g.published, member)
			if err := g.answers[member]; err != nil {
				return g.said[member], err
			}
			g.held[member] = true
			return "", nil
		},
		pause:    func(time.Duration) { g.slept++ },
		out:      &log,
		verify:   true,
		patience: time.Minute,
		interval: time.Second,
	}
}

func newRegistry() *registry {
	return &registry{held: map[string]bool{}, answers: map[string]error{}, said: map[string]string{}}
}

func TestTheLauncherIsPublishedAfterEveryBinaryItPins(t *testing.T) {
	got := npmMembers(npmpkg.CLI)
	if len(got) != len(npmpkg.Platforms())+1 {
		t.Fatalf("members = %v", got)
	}
	if last := got[len(got)-1]; last != npmpkg.CLI {
		t.Errorf("last member = %q, want the launcher; it pins the others and must never precede them", last)
	}
	for _, member := range got[:len(got)-1] {
		if member == npmpkg.CLI {
			t.Errorf("the launcher appears before the binaries in %v", got)
		}
	}
}

func TestAPackageWithNoBinariesIsPublishedOnItsOwn(t *testing.T) {
	if got := npmMembers("@apptivitypl/something"); len(got) != 1 || got[0] != "@apptivitypl/something" {
		t.Errorf("members = %v, want just the package", got)
	}
}

func TestEverythingIsPublishedInOrder(t *testing.T) {
	held := newRegistry()
	if err := held.job(t).run(npmMembers(npmpkg.CLI)); err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(held.published) != len(npmpkg.Platforms())+1 {
		t.Fatalf("published = %v", held.published)
	}
	if last := held.published[len(held.published)-1]; last != npmpkg.CLI {
		t.Errorf("published = %v, want the launcher last", held.published)
	}
}

func TestTheLauncherIsHeldBackWhileABinaryIsStillUnderReview(t *testing.T) {
	held := newRegistry()
	job := held.job(t)
	stuck := npmpkg.Platforms()[0].Package()
	job.present = func(member string) (bool, error) {
		if member == stuck {
			return false, nil
		}
		return held.held[member], nil
	}
	err := job.run(npmMembers(npmpkg.CLI))
	if err == nil {
		t.Fatal("the launcher must not go out while a binary it pins is missing")
	}
	if !strings.Contains(err.Error(), stuck) {
		t.Errorf("err = %v, want %s named", err, stuck)
	}
	for _, member := range held.published {
		if member == npmpkg.CLI {
			t.Error("the launcher was published anyway")
		}
	}
}

func TestARefusalBecauseTheVersionIsAlreadyThereIsNotAFailure(t *testing.T) {
	held := newRegistry()
	stuck := npmpkg.Platforms()[0].Package()
	held.answers[stuck] = errors.New("exit status 1")
	held.said[stuck] = "npm error code E409\nnpm error 409 Conflict - Cannot publish over previously staged version"
	job := held.job(t)
	job.verify = false
	if err := job.run(npmMembers(npmpkg.CLI)); err != nil {
		t.Fatalf("a version already with the registry must not stop the release: %v", err)
	}
	if last := held.published[len(held.published)-1]; last != npmpkg.CLI {
		t.Errorf("published = %v, want the run to have carried on to the launcher", held.published)
	}
}

func TestAnyOtherRefusalStopsTheRelease(t *testing.T) {
	held := newRegistry()
	stuck := npmpkg.Platforms()[0].Package()
	held.answers[stuck] = errors.New("exit status 1")
	held.said[stuck] = "npm error code E403\nnpm error 403 Forbidden"
	err := held.job(t).run(npmMembers(npmpkg.CLI))
	if err == nil {
		t.Fatal("a refusal that is not about the version already existing must stop the release")
	}
	if len(held.published) != 1 {
		t.Errorf("published = %v, want the run to have stopped at the first refusal", held.published)
	}
}

func TestAMemberAlreadyOnTheRegistryIsNotPublishedAgain(t *testing.T) {
	held := newRegistry()
	skipped := npmpkg.Platforms()[1].Package()
	held.held[skipped] = true
	if err := held.job(t).run(npmMembers(npmpkg.CLI)); err != nil {
		t.Fatalf("run: %v", err)
	}
	for _, member := range held.published {
		if member == skipped {
			t.Errorf("%s was published again", skipped)
		}
	}
}

func TestWaitingGivesUpWithSomethingActionable(t *testing.T) {
	held := newRegistry()
	job := held.job(t)
	job.patience = 2 * time.Second
	job.interval = time.Second
	job.present = func(string) (bool, error) { return false, nil }
	err := job.settle([]string{"@apptivitypl/gopage-linux-x64"})
	if err == nil {
		t.Fatal("settle must give up rather than wait forever")
	}
	for _, want := range []string{"gopage-linux-x64", "run this again"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("err = %v, want %q in it", err, want)
		}
	}
	if held.slept == 0 {
		t.Error("settle returned without ever waiting")
	}
}

func TestWaitingStopsAsSoonAsEverythingIsThere(t *testing.T) {
	held := newRegistry()
	job := held.job(t)
	job.present = func(string) (bool, error) { return true, nil }
	if err := job.settle([]string{"@apptivitypl/gopage-linux-x64"}); err != nil {
		t.Fatalf("settle: %v", err)
	}
	if held.slept != 0 {
		t.Errorf("slept %d times, want none", held.slept)
	}
}

func TestARegistryThatCannotBeReachedIsReported(t *testing.T) {
	held := newRegistry()
	job := held.job(t)
	job.present = func(string) (bool, error) { return false, errors.New("no network") }
	if err := job.run(npmMembers(npmpkg.CLI)); err == nil {
		t.Error("a registry that cannot be asked must stop the release")
	}
	if _, err := job.absent([]string{"x"}); err == nil {
		t.Error("absent must report the failure too")
	}
}

func TestAnEmptyListPublishesNothing(t *testing.T) {
	held := newRegistry()
	if err := held.job(t).run(nil); err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(held.published) != 0 {
		t.Errorf("published = %v, want nothing", held.published)
	}
}

func TestAnAssemblyThatFailsStopsTheRelease(t *testing.T) {
	held := newRegistry()
	job := held.job(t)
	job.assemble = func(string) (string, error) { return "", errors.New("no archive") }
	if err := job.run(npmMembers(npmpkg.CLI)); err == nil {
		t.Error("a package that cannot be assembled must stop the release")
	}
}
