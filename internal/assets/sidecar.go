package assets

import (
	"io/fs"
	"path"
	"sort"
	"strings"
)

type Sidecar struct {
	Links   []string
	Modules []string
	Islands map[string][]string
}

func ReadSidecar(fsys fs.FS) Sidecar {
	if fsys == nil {
		return Sidecar{}
	}
	data, err := fs.ReadFile(fsys, path.Join(BundleDir, PreloadFile))
	if err != nil {
		return Sidecar{}
	}
	return ParseSidecar(data)
}

func ParseSidecar(data []byte) Sidecar {
	sidecar := Sidecar{Islands: map[string][]string{}}
	for line := range strings.SplitSeq(string(data), "\n") {
		kind, rest, _ := strings.Cut(strings.TrimSpace(line), " ")
		switch kind {
		case "link":
			sidecar.Links = append(sidecar.Links, rest)
		case "module":
			sidecar.Modules = append(sidecar.Modules, rest)
		case "island":
			name, chunks, _ := strings.Cut(rest, " ")
			sidecar.Islands[name] = strings.Fields(chunks)
		}
	}
	return sidecar
}

func (s Sidecar) Bytes() []byte {
	var b strings.Builder
	for _, link := range s.Links {
		b.WriteString("link " + link + "\n")
	}
	for _, module := range s.Modules {
		b.WriteString("module " + module + "\n")
	}
	names := make([]string, 0, len(s.Islands))
	for name := range s.Islands {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		b.WriteString("island " + name + " " + strings.Join(s.Islands[name], " ") + "\n")
	}
	return []byte(b.String())
}

func (s Sidecar) modules() map[string]bool {
	eager := make(map[string]bool, len(s.Modules))
	for _, name := range s.Modules {
		eager[name] = true
	}
	return eager
}

func (s Sidecar) Link() string {
	return strings.Join(s.Links, ", ")
}

func (s Sidecar) IslandChunks() map[string][]string {
	eager := s.modules()
	lazy := make(map[string][]string, len(s.Islands))
	for name, chunks := range s.Islands {
		var kept []string
		for _, chunk := range chunks {
			if !eager[chunk] {
				kept = append(kept, chunk)
			}
		}
		if len(kept) > 0 {
			lazy[name] = kept
		}
	}
	return lazy
}
