package main

import (
	"fmt"
	"io"
	"strings"
	"time"
)

const (
	reviewPatience = 20 * time.Minute
	reviewInterval = 15 * time.Second
	alreadyThere   = "E409"
)

type npmRelease struct {
	version  string
	assemble func(member string) (string, error)
	present  func(member string) (bool, error)
	publish  func(dir string) (string, error)
	pause    func(time.Duration)
	out      io.Writer
	verify   bool
	patience time.Duration
	interval time.Duration
}

func (r npmRelease) say(format string, args ...any) {
	_, _ = fmt.Fprintf(r.out, format, args...)
}

func (r npmRelease) run(members []string) error {
	if len(members) == 0 {
		return nil
	}
	binaries, launcher := members[:len(members)-1], members[len(members)-1]
	for _, member := range binaries {
		if err := r.one(member); err != nil {
			return err
		}
	}
	if r.verify {
		if err := r.settle(binaries); err != nil {
			return err
		}
	}
	return r.one(launcher)
}

func (r npmRelease) one(member string) error {
	on, err := r.present(member)
	if err != nil {
		return err
	}
	if on {
		r.say("%s %s is already on the registry\n", member, r.version)
		return nil
	}
	dir, err := r.assemble(member)
	if err != nil {
		return err
	}
	r.say("assembled %s %s in %s\n", member, r.version, dir)
	said, err := r.publish(dir)
	if err == nil {
		return nil
	}
	if strings.Contains(said, alreadyThere) {
		r.say("%s %s is already with the registry, waiting on its review\n", member, r.version)
		return nil
	}
	return err
}

func (r npmRelease) settle(members []string) error {
	for attempt := 0; ; attempt++ {
		waiting, err := r.absent(members)
		if err != nil {
			return err
		}
		if len(waiting) == 0 {
			return nil
		}
		if time.Duration(attempt)*r.interval >= r.patience {
			return fmt.Errorf("the registry has not finished reviewing %s after %s\n"+
				"nothing is lost and nothing is half published: run this again once they appear, "+
				"and the release will carry on from here",
				strings.Join(waiting, ", "), r.patience)
		}
		if attempt == 0 {
			r.say("waiting for the registry to publish %s\n", strings.Join(waiting, ", "))
		}
		r.pause(r.interval)
	}
}

func (r npmRelease) absent(members []string) ([]string, error) {
	var waiting []string
	for _, member := range members {
		on, err := r.present(member)
		if err != nil {
			return nil, err
		}
		if !on {
			waiting = append(waiting, member)
		}
	}
	return waiting, nil
}
