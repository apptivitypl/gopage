package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/apptivitypl/rill/internal/jsonc"
)

const FileName = "dev.jsonc"

type Mode string

const (
	ModeRatchet Mode = "ratchet"
	ModeFixed   Mode = "fixed"
)

type file struct {
	Coverage global `json:"coverage"`
}

type global struct {
	GlobalStatements  float64                `json:"globalStatements"`
	Mode              Mode                   `json:"mode"`
	Lock              string                 `json:"lock"`
	DiffStatements    float64                `json:"diffStatements"`
	RatchetTolerance  float64                `json:"ratchetTolerance"`
	StubMinStatements int                    `json:"stubMinStatements"`
	Exclude           []string               `json:"exclude"`
	Packages          map[string]PackageRule `json:"packages"`
}

type PackageRule struct {
	Statements    float64 `json:"statements"`
	Justification string  `json:"justification,omitempty"`
}

type Config struct {
	GlobalStatements  float64
	Mode              Mode
	Lock              string
	DiffStatements    float64
	RatchetTolerance  float64
	StubMinStatements int
	Packages          map[string]PackageRule

	exclude []string
}

func Parse(text string) (*Config, error) {
	plain, err := jsonc.ToJSON([]byte(text))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", FileName, err)
	}
	var f file
	decoder := json.NewDecoder(bytes.NewReader(plain))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&f); err != nil {
		return nil, fmt.Errorf("%s: %w", FileName, err)
	}
	applyDefaults(&f)
	if err := validateJustifications(&f); err != nil {
		return nil, err
	}
	if err := validatePatterns(f.Coverage.Exclude); err != nil {
		return nil, err
	}
	return &Config{
		GlobalStatements:  f.Coverage.GlobalStatements,
		Mode:              f.Coverage.Mode,
		Lock:              f.Coverage.Lock,
		DiffStatements:    f.Coverage.DiffStatements,
		RatchetTolerance:  f.Coverage.RatchetTolerance,
		StubMinStatements: f.Coverage.StubMinStatements,
		Packages:          f.Coverage.Packages,
		exclude:           f.Coverage.Exclude,
	}, nil
}

func applyDefaults(f *file) {
	if f.Coverage.Mode == "" {
		f.Coverage.Mode = ModeRatchet
	}
	if f.Coverage.Lock == "" {
		f.Coverage.Lock = "dev.lock.json"
	}
	if f.Coverage.DiffStatements == 0 {
		f.Coverage.DiffStatements = f.Coverage.GlobalStatements
	}
	if f.Coverage.RatchetTolerance == 0 {
		f.Coverage.RatchetTolerance = 0.1
	}
	if f.Coverage.StubMinStatements == 0 {
		f.Coverage.StubMinStatements = 10
	}
}

func validateJustifications(f *file) error {
	for name, rule := range f.Coverage.Packages {
		if rule.Statements >= f.Coverage.GlobalStatements {
			continue
		}
		if strings.TrimSpace(rule.Justification) == "" {
			return fmt.Errorf(
				"%s: package %q sets threshold %.1f%% below the global %.1f%% without a justification; every exemption must be signed",
				FileName, name, rule.Statements, f.Coverage.GlobalStatements)
		}
	}
	return nil
}

func validatePatterns(patterns []string) error {
	for _, p := range patterns {
		if !doublestar.ValidatePattern(p) {
			return fmt.Errorf("%s: invalid exclude pattern %q", FileName, p)
		}
	}
	return nil
}

func (c *Config) Threshold(pkg string) float64 {
	if rule, ok := c.Packages[pkg]; ok {
		return rule.Statements
	}
	return c.GlobalStatements
}

func (c *Config) IsExcluded(path string) bool {
	for _, pattern := range c.exclude {
		if ok, err := doublestar.Match(pattern, path); err == nil && ok {
			return true
		}
	}
	return false
}
