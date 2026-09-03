package covprofile

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type Block struct {
	File      string
	StartLine int
	EndLine   int
	NumStmts  int
	Count     int
}

type Coverage struct {
	Total   int
	Covered int
}

func (c Coverage) Percent() float64 {
	if c.Total == 0 {
		return 100
	}
	return float64(c.Covered) * 100 / float64(c.Total)
}

func (c *Coverage) add(other Coverage) {
	c.Total += other.Total
	c.Covered += other.Covered
}

type FileCoverage struct {
	Path string
	Coverage
}

type Report struct {
	Packages map[string]Coverage
	Files    []FileCoverage
	Blocks   []Block
	Total    Coverage
}

type Excluder interface {
	IsExcluded(path string) bool
}

func Parse(profile string) ([]Block, error) {
	var blocks []Block
	for i, line := range strings.Split(profile, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "mode:") {
			continue
		}
		block, err := parseBlock(line)
		if err != nil {
			return nil, fmt.Errorf("coverage profile line %d: %w", i+1, err)
		}
		blocks = append(blocks, block)
	}
	return blocks, nil
}

func parseBlock(line string) (Block, error) {
	sep := strings.LastIndex(line, ":")
	if sep < 0 {
		return Block{}, fmt.Errorf("missing file separator in %q", line)
	}
	file := line[:sep]
	fields := strings.Fields(line[sep+1:])
	if len(fields) != 3 {
		return Block{}, fmt.Errorf("want 3 fields after the position, got %d in %q", len(fields), line)
	}
	startLine, endLine, err := parseRange(fields[0])
	if err != nil {
		return Block{}, err
	}
	numStmts, err := strconv.Atoi(fields[1])
	if err != nil {
		return Block{}, fmt.Errorf("statement count: %w", err)
	}
	count, err := strconv.Atoi(fields[2])
	if err != nil {
		return Block{}, fmt.Errorf("hit count: %w", err)
	}
	return Block{File: file, StartLine: startLine, EndLine: endLine, NumStmts: numStmts, Count: count}, nil
}

func parseRange(spec string) (int, int, error) {
	from, to, ok := strings.Cut(spec, ",")
	if !ok {
		return 0, 0, fmt.Errorf("malformed position %q", spec)
	}
	startLine, err := parsePosition(from)
	if err != nil {
		return 0, 0, err
	}
	endLine, err := parsePosition(to)
	if err != nil {
		return 0, 0, err
	}
	return startLine, endLine, nil
}

func parsePosition(spec string) (int, error) {
	lineText, _, ok := strings.Cut(spec, ".")
	if !ok {
		return 0, fmt.Errorf("malformed position %q", spec)
	}
	line, err := strconv.Atoi(lineText)
	if err != nil {
		return 0, fmt.Errorf("line number in %q: %w", spec, err)
	}
	return line, nil
}

func Relativize(file, modulePath string) string {
	return strings.TrimPrefix(strings.TrimPrefix(file, modulePath), "/")
}

const RootPackage = "."

func PackageOf(relPath string) string {
	cut := strings.LastIndex(relPath, "/")
	if cut < 0 {
		return RootPackage
	}
	return relPath[:cut]
}

func Aggregate(blocks []Block, modulePath string, exclude Excluder) Report {
	report := Report{Packages: map[string]Coverage{}}
	files := map[string]Coverage{}

	for _, block := range blocks {
		rel := Relativize(block.File, modulePath)
		if exclude != nil && exclude.IsExcluded(rel) {
			continue
		}
		pkg := PackageOf(rel)
		cov := Coverage{Total: block.NumStmts}
		if block.Count > 0 {
			cov.Covered = block.NumStmts
		}
		pkgCov := report.Packages[pkg]
		pkgCov.add(cov)
		report.Packages[pkg] = pkgCov

		fileCov := files[rel]
		fileCov.add(cov)
		files[rel] = fileCov

		report.Total.add(cov)
		block.File = rel
		report.Blocks = append(report.Blocks, block)
	}

	for path, cov := range files {
		report.Files = append(report.Files, FileCoverage{Path: path, Coverage: cov})
	}
	sort.Slice(report.Files, func(i, j int) bool {
		if report.Files[i].Percent() != report.Files[j].Percent() {
			return report.Files[i].Percent() < report.Files[j].Percent()
		}
		return report.Files[i].Path < report.Files[j].Path
	})
	return report
}
