package assets

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"mime"
	"os"
	"path"
	"sort"
	"strings"

	"github.com/sonquer/rill/internal/paths"
)

const (
	Dir    = paths.StylesDir
	Prefix = "/assets/"
)

type Kind uint8

const (
	KindOther Kind = iota
	KindStyle
	KindScript
	KindModule
	KindFont
)

type Asset struct {
	Source string
	Path   string
	Kind   Kind
	ETag   string
	Type   string
	Size   int64
	Cache  string
	Inline string
}

type Transform func(Asset, []byte) ([]byte, error)

type Inliner func(Asset, []byte) bool

type Options struct {
	Transform Transform
	Inline    Inliner
}

func Collect(fsys fs.FS) ([]Asset, error) {
	return CollectWith(fsys, nil)
}

func CollectWith(fsys fs.FS, transform Transform) ([]Asset, error) {
	return CollectOptions(fsys, Options{Transform: transform})
}

func CollectOptions(fsys fs.FS, opts Options) ([]Asset, error) {
	transform := opts.Transform
	var list []Asset
	err := fs.WalkDir(fsys, Dir, func(file string, entry fs.DirEntry, err error) error {
		if err != nil {
			if file == Dir {
				return fs.SkipAll
			}
			return err
		}
		if entry.IsDir() || strings.HasSuffix(file, BrotliSuffix) || strings.HasSuffix(file, GzipSuffix) {
			return nil
		}
		data, err := fs.ReadFile(fsys, file)
		if err != nil {
			return err
		}
		asset := describe(file, data)
		if transform != nil {
			shaped, err := transform(asset, data)
			if err != nil {
				return err
			}
			asset = describe(file, shaped)
			data = shaped
		}
		if opts.Inline != nil && asset.Kind == KindStyle && opts.Inline(asset, data) {
			asset.Inline = string(data)
		}
		list = append(list, asset)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Source < list[j].Source })
	return list, nil
}

const BundleDir = "bundles"

func Describe(name string, data []byte) Asset {
	sum := sha256.Sum256(data)
	extension := path.Ext(name)
	return Asset{
		Source: BundleDir + "/" + name,
		Path:   Prefix + name,
		Kind:   kindOf(extension),
		ETag:   `"` + hex.EncodeToString(sum[:8]) + `"`,
		Type:   contentType(extension),
		Size:   int64(len(data)),
	}
}

const (
	PublicDir     = paths.PublicDir
	PublicCache   = "public, max-age=3600"
	PublicRefresh = "no-cache"
	DevVar        = "RILL_DEV"
)

func publicCache() string {
	if os.Getenv(DevVar) != "" {
		return PublicRefresh
	}
	return PublicCache
}

func Public(fsys fs.FS) ([]Asset, error) {
	var list []Asset
	err := fs.WalkDir(fsys, PublicDir, func(file string, entry fs.DirEntry, err error) error {
		if err != nil {
			if file == PublicDir {
				return fs.SkipAll
			}
			return err
		}
		if entry.IsDir() {
			return nil
		}
		data, err := fs.ReadFile(fsys, file)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		extension := path.Ext(file)
		list = append(list, Asset{
			Source: file,
			Path:   strings.TrimPrefix(file, PublicDir),
			Kind:   kindOf(extension),
			ETag:   `"` + hex.EncodeToString(sum[:8]) + `"`,
			Type:   contentType(extension),
			Size:   int64(len(data)),
			Cache:  publicCache(),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Source < list[j].Source })
	return list, nil
}

const (
	PreloadFile   = "rill.preload"
	RuntimePrefix = "rill.client."
)

func Verbatim(fsys fs.FS) ([]Asset, error) {
	entries, err := fs.ReadDir(fsys, BundleDir)
	if err != nil {
		return nil, nil
	}
	eager := ReadSidecar(fsys).modules()
	var list []Asset
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || name == PreloadFile ||
			strings.HasSuffix(name, BrotliSuffix) || strings.HasSuffix(name, GzipSuffix) {
			continue
		}
		data, err := fs.ReadFile(fsys, path.Join(BundleDir, name))
		if err != nil {
			return nil, err
		}
		asset := Describe(name, data)
		asset.Kind = bundleKind(name, eager)
		list = append(list, asset)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Source < list[j].Source })
	return list, nil
}

func bundleKind(name string, eager map[string]bool) Kind {
	switch {
	case strings.HasPrefix(name, RuntimePrefix):
		return KindScript
	case eager[name]:
		return KindModule
	default:
		return KindOther
	}
}

func describe(file string, data []byte) Asset {
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:8])
	relative := strings.TrimPrefix(strings.TrimPrefix(file, Dir), "/")
	extension := path.Ext(relative)
	stem := strings.TrimSuffix(relative, extension)
	return Asset{
		Source: file,
		Path:   Prefix + stem + "." + hash + extension,
		Kind:   kindOf(extension),
		ETag:   `"` + hash + `"`,
		Type:   contentType(extension),
		Size:   int64(len(data)),
	}
}

func kindOf(extension string) Kind {
	switch extension {
	case ".css":
		return KindStyle
	case ".js", ".mjs":
		return KindScript
	case ".woff2", ".woff":
		return KindFont
	default:
		return KindOther
	}
}

func contentType(extension string) string {
	if known := mime.TypeByExtension(extension); known != "" {
		return known
	}
	return "application/octet-stream"
}

func Tags(list []Asset) string {
	var b strings.Builder
	for _, asset := range list {
		if asset.Kind == KindStyle && asset.Inline != "" {
			b.WriteString("<style>" + asset.Inline + "</style>")
		}
	}
	for _, asset := range list {
		if asset.Kind == KindStyle && asset.Inline == "" {
			b.WriteString(`<link rel="stylesheet" href="` + asset.Path + `">`)
		}
	}
	for _, asset := range list {
		switch asset.Kind {
		case KindScript:
			b.WriteString(`<script type="module" async src="` + asset.Path + `"></script>`)
		case KindModule:
			b.WriteString(`<link rel="modulepreload" href="` + asset.Path + `">`)
		case KindStyle, KindOther:
		}
	}
	return b.String()
}

func Link(list []Asset) string {
	var parts []string
	for _, asset := range list {
		switch asset.Kind {
		case KindStyle:
			if asset.Inline == "" {
				parts = append(parts, "<"+asset.Path+`>; rel=preload; as=style`)
			}
		case KindScript, KindModule:
			parts = append(parts, "<"+asset.Path+`>; rel=modulepreload`)
		case KindFont:
			parts = append(parts, "<"+asset.Path+`>; rel=preload; as=font; crossorigin`)
		case KindOther:
		}
	}
	return strings.Join(parts, ", ")
}
