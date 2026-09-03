package release

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/apptivitypl/rill/internal/tool/shell"
)

const registryHost = "https://registry.npmjs.org"

var client = &http.Client{Timeout: 20 * time.Second}

func Published(pkg Package) (bool, error) {
	switch pkg.Kind {
	case KindGo:
		return Tagged(pkg.TagName())
	case KindNPM:
		return onRegistry(pkg.Name, pkg.Version)
	default:
		return false, fmt.Errorf("%s: package %q has kind %q, which nothing knows how to publish yet", FileName, pkg.Name, pkg.Kind)
	}
}

func Changed(pkg Package) (bool, error) {
	tag := pkg.TagName()
	tagged, err := Tagged(tag)
	if err != nil {
		return false, err
	}
	if !tagged {
		return false, nil
	}
	out, err := shell.Capture("git", "diff", "--name-only", tag+"..HEAD")
	if err != nil {
		return false, err
	}
	for _, path := range strings.Fields(out) {
		if pkg.Owns(path) {
			return true, nil
		}
	}
	return false, nil
}

func Tagged(tag string) (bool, error) {
	local, err := shell.Capture("git", "tag", "--list", tag)
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(local) != "" {
		return true, nil
	}
	remote, err := shell.Capture("git", "ls-remote", "--tags", "origin", "refs/tags/"+tag)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(remote) != "", nil
}

func onRegistry(name, version string) (bool, error) {
	address := fmt.Sprintf("%s/%s/%s", registryHost, url.PathEscape(name), url.PathEscape(version))
	response, err := client.Get(address)
	if err != nil {
		return false, fmt.Errorf("ask %s about %s@%s: %w", registryHost, name, version, err)
	}
	defer func() { _ = response.Body.Close() }()
	switch response.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		return false, fmt.Errorf("%s answered %s for %s@%s", registryHost, response.Status, name, version)
	}
}
