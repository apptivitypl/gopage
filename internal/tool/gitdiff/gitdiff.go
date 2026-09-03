package gitdiff

import (
	"strconv"
	"strings"

	"github.com/sonquer/rill/internal/tool/covprofile"
)

type ChangedLines map[string]map[int]bool

func ParseUnified(diff string) ChangedLines {
	changed := ChangedLines{}
	var current string

	for line := range strings.SplitSeq(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "+++ "):
			current = targetPath(strings.TrimSpace(strings.TrimPrefix(line, "+++ ")))
		case strings.HasPrefix(line, "@@") && current != "":
			start, count, ok := hunkRange(line)
			if !ok || count == 0 {
				continue
			}
			if changed[current] == nil {
				changed[current] = map[int]bool{}
			}
			for i := range count {
				changed[current][start+i] = true
			}
		}
	}
	return changed
}

func targetPath(spec string) string {
	if spec == "/dev/null" {
		return ""
	}
	return strings.TrimPrefix(spec, "b/")
}

func hunkRange(header string) (int, int, bool) {
	_, after, ok := strings.Cut(header, "+")
	if !ok {
		return 0, 0, false
	}
	spec := strings.Fields(after)
	if len(spec) == 0 {
		return 0, 0, false
	}
	startText, countText, hasCount := strings.Cut(spec[0], ",")
	start, err := strconv.Atoi(startText)
	if err != nil {
		return 0, 0, false
	}
	count := 1
	if hasCount {
		if count, err = strconv.Atoi(countText); err != nil {
			return 0, 0, false
		}
	}
	return start, count, true
}

func Coverage(blocks []covprofile.Block, changed ChangedLines) (int, int) {
	var covered, total int
	for path, lines := range changed {
		for line := range lines {
			block, ok := innermostBlock(blocks, path, line)
			if !ok {
				continue
			}
			total++
			if block.Count > 0 {
				covered++
			}
		}
	}
	return covered, total
}

func innermostBlock(blocks []covprofile.Block, path string, line int) (covprofile.Block, bool) {
	var best covprofile.Block
	var found bool
	for _, block := range blocks {
		if block.File != path || line < block.StartLine || line > block.EndLine {
			continue
		}
		if !found || span(block) < span(best) {
			best, found = block, true
		}
	}
	return best, found
}

func span(b covprofile.Block) int {
	return b.EndLine - b.StartLine
}
