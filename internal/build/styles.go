package build

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/sonquer/rill/internal/assets"
	"github.com/sonquer/rill/internal/config"
	"github.com/sonquer/rill/internal/css"
	"github.com/sonquer/rill/internal/paths"
)

type styles struct {
	processor css.Processor
	dir       string
	done      map[string][]byte
}

func stylesheet(opts Options, settings config.Config) styles {
	if settings.CSS.Engine != config.EngineTailwind {
		return styles{}
	}
	processor := opts.Styles
	if processor == nil {
		processor = css.Tailwind{Fetch: css.Download, Minify: true}
	}
	return styles{processor: processor, dir: opts.Dir, done: map[string][]byte{}}
}

func (s styles) transform(asset assets.Asset, content []byte) ([]byte, error) {
	if asset.Kind != assets.KindStyle {
		return content, nil
	}
	if s.processor == nil {
		return minified(asset, content)
	}
	if held, ok := s.done[asset.Source]; ok {
		return held, nil
	}
	work := filepath.Join(s.dir, filepath.FromSlash(paths.CacheDir), "styles")
	if err := os.MkdirAll(work, 0o755); err != nil {
		return nil, err
	}
	input := filepath.Join(s.dir, filepath.FromSlash(asset.Source))
	output := filepath.Join(work, filepath.Base(asset.Source))
	inventory := filepath.Join(s.dir, filepath.FromSlash(paths.Inventory))
	if err := s.processor.Process(input, output, inventory); err != nil {
		return nil, fmt.Errorf("%s: %w", asset.Source, err)
	}
	processed, err := os.ReadFile(output)
	if err != nil {
		return nil, err
	}
	s.done[asset.Source] = processed
	return processed, nil
}
