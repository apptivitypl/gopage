package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/sonquer/rill/internal/compile"
	"github.com/sonquer/rill/internal/config"
	"github.com/sonquer/rill/internal/diag"
	"github.com/sonquer/rill/internal/i18n"
)

func i18nCommand(args []string) error {
	if len(args) == 0 {
		return errors.New("missing subcommand\n\n" + i18nUsage())
	}
	var dir string
	fs := flag.NewFlagSet("i18n", flag.ContinueOnError)
	fs.StringVar(&dir, "dir", ".", "project directory")
	rest, err := parseInterleaved(fs, args[1:])
	if err != nil {
		return err
	}
	if len(rest) > 0 {
		return fmt.Errorf("unexpected argument %q\n\n%s", rest[0], i18nUsage())
	}

	audit, err := auditProject(dir)
	if err != nil {
		return err
	}
	switch args[0] {
	case "coverage":
		return reportCoverage(audit)
	case "orphans":
		return reportOrphans(audit)
	case "extract":
		return reportExtract(audit)
	case "sync":
		return reportSync(audit)
	default:
		return fmt.Errorf("unknown subcommand %q\n\n%s", args[0], i18nUsage())
	}
}

func i18nUsage() string {
	return strings.Join([]string{
		"subcommands:",
		"  i18n coverage [--dir DIR]   how much of every locale is translated",
		"  i18n extract [--dir DIR]    every key the templates ask for",
		"  i18n sync [--dir DIR]       json for the keys a locale is missing",
		"  i18n orphans [--dir DIR]    catalog keys no template uses",
	}, "\n")
}

type i18nAudit struct {
	keys     []string
	catalogs map[string]i18n.Catalog
	settings config.Config
	reports  []i18n.Report
}

func auditProject(dir string) (i18nAudit, error) {
	fsys := os.DirFS(dir)
	settings, err := config.Load(fsys)
	if err != nil {
		return i18nAudit{}, err
	}
	catalogs, err := i18n.Load(fsys)
	if err != nil {
		return i18nAudit{}, err
	}
	var bag diag.Bag
	result, err := compile.Compile(fsys, &bag)
	if err != nil {
		return i18nAudit{}, err
	}
	return i18nAudit{
		keys:     result.Messages,
		catalogs: catalogs,
		settings: settings,
		reports:  i18n.Audit(result.Messages, catalogs, settings.I18n.Locales),
	}, nil
}

func reportCoverage(audit i18nAudit) error {
	fmt.Printf("%-8s %8s %8s %s\n", "locale", "keys", "done", "status")
	incomplete := 0
	for _, report := range audit.reports {
		status := "ok"
		if !report.Complete() {
			status = fmt.Sprintf("%d missing", len(report.Missing))
			incomplete++
		}
		fmt.Printf("%-8s %8d %7.1f%% %s\n", report.Locale, report.Keys, report.Percent(), status)
	}
	if incomplete > 0 {
		return fmt.Errorf("%d of %d locales are incomplete", incomplete, len(audit.reports))
	}
	return nil
}

func reportOrphans(audit i18nAudit) error {
	total := 0
	for _, report := range audit.reports {
		for _, key := range report.Orphans {
			fmt.Printf("%s: %s\n", report.Locale, key)
			total++
		}
	}
	if total == 0 {
		fmt.Println("every catalog key is used by a template")
	}
	return nil
}

func reportExtract(audit i18nAudit) error {
	for _, key := range audit.keys {
		fmt.Println(key)
	}
	if len(audit.keys) == 0 {
		fmt.Println("no template asks for a message")
	}
	return nil
}

func reportSync(audit i18nAudit) error {
	source := audit.catalogs[audit.settings.I18n.DefaultLocale]
	for _, report := range audit.reports {
		snippet := i18n.Snippet(report.Missing, source)
		if snippet == "" {
			continue
		}
		fmt.Printf("// locales/%s.json\n%s\n", report.Locale, snippet)
	}
	return nil
}
