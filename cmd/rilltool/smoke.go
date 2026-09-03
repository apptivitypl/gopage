package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/andybalholm/brotli"

	"github.com/sonquer/rill/internal/assets"
	"github.com/sonquer/rill/internal/build"
	"github.com/sonquer/rill/internal/bundle"
	"github.com/sonquer/rill/internal/paths"
	"github.com/sonquer/rill/internal/tool/benchlog"
	"github.com/sonquer/rill/internal/tool/smoke"
)

const (
	smokeModule   = "example.com/smoke"
	smokeTemplate = "hello-world"
	readyAttempts = 60
	readyPause    = time.Second
)

func smokeCmd(args []string) error {
	var keep, record, reference bool
	fs := flag.NewFlagSet("smoke", flag.ContinueOnError)
	fs.BoolVar(&keep, "keep", false, "leave the generated project on disk")
	fs.BoolVar(&record, "record", false, "write the module size into the performance log")
	fs.BoolVar(&reference, "reference", false, "run the reference application on both adapters")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if _, err := exec.LookPath("wrangler"); err != nil {
		return errors.New("wrangler is not on PATH; run pnpm install and add node_modules/.bin to PATH")
	}
	root, err := repoRoot()
	if err != nil {
		return err
	}

	dir, err := os.MkdirTemp("", "rill-smoke-")
	if err != nil {
		return err
	}
	if !keep {
		defer func() { _ = os.RemoveAll(dir) }()
	}
	project := filepath.Join(dir, "app")

	template := smokeTemplate
	locales := "en"
	if reference {
		template = smoke.ReferenceTemplate
		locales = "en,pl"
	}
	if err := scaffoldProject(root, project, template, locales); err != nil {
		return err
	}
	if err := buildProject(root, project, string(build.TargetNative)); err != nil {
		return err
	}
	if reference {
		return runReference(project, keep)
	}
	if err := probeNative(project); err != nil {
		return err
	}
	if err := probeDev(root, project); err != nil {
		return err
	}
	if err := buildProject(root, project, string(build.TargetWorkers)); err != nil {
		return err
	}
	size, err := reportWorkerSize(project)
	if err != nil {
		return err
	}
	runtime, chunks, err := reportClientSize(project)
	if err != nil {
		return err
	}
	if record {
		if err := recordSizes(root, size, runtime, chunks); err != nil {
			return err
		}
	} else if err := checkClientSize(root, runtime, chunks); err != nil {
		return err
	}
	return serveAndProbe(project, keep)
}

func scaffoldProject(root, project, template, locales string) error {
	steps := []struct {
		dir  string
		name string
		args []string
	}{
		{root, "go", []string{"run", "./cmd/rill", "new", project,
			"--module", smokeModule, "--template", template, "--rill-path", root,
			"--locales", locales, "--nav", "partial", "--yes"}},
	}
	for _, step := range steps {
		cmd := exec.Command(step.name, step.args...)
		cmd.Dir = step.dir
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("%s %v: %w", step.name, step.args, err)
		}
	}
	return nil
}

func buildProject(root, project, target string) error {
	cmd := exec.Command("go", "run", "./cmd/rill", "build", "--dir", project, "--target", target)
	cmd.Dir = root
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func probeNative(project string) error {
	port, err := smoke.FreePort()
	if err != nil {
		return err
	}
	cmd := exec.Command(filepath.Join(project, filepath.FromSlash(paths.NativeBinary)))
	cmd.Dir = project
	cmd.Env = append(os.Environ(), fmt.Sprintf("ADDR=:%d", port))
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()

	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	fetch := smokeFetcher()
	if err := smoke.WaitReady(fetch, base, readyAttempts, readyPause); err != nil {
		return fmt.Errorf("native binary: %w", err)
	}
	if err := smoke.Run(fetch, base); err != nil {
		return fmt.Errorf("native binary: %w", err)
	}
	fmt.Println("smoke: ok — the native binary answered every check")
	return nil
}

func smokeFetcher() smoke.Fetcher {
	return smoke.HTTPFetcher(smokeClient())
}

func smokeClient() *http.Client {
	return &http.Client{
		Timeout:       10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

func reportWorkerSize(project string) (int, error) {
	path := filepath.Join(project, filepath.FromSlash(paths.WorkerBinary))
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer func() { _ = file.Close() }()

	counter := &countingWriter{}
	writer := brotli.NewWriterLevel(counter, brotli.DefaultCompression)
	if _, err := io.Copy(writer, file); err != nil {
		return 0, err
	}
	if err := writer.Close(); err != nil {
		return 0, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	fmt.Printf("worker module: %d bytes, %d after brotli (budget %d)\n",
		info.Size(), counter.n, smoke.WorkerSizeBudget)
	return counter.n, smoke.CheckSize(counter.n)
}

func reportClientSize(project string) (int, int, error) {
	entries, err := os.ReadDir(filepath.Join(project, filepath.FromSlash(paths.GenBundles)))
	if err != nil {
		return 0, 0, err
	}
	var runtime, chunks int
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, assets.BrotliSuffix) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return 0, 0, err
		}
		if strings.HasPrefix(name, bundle.RuntimePrefix) {
			runtime += int(info.Size())
		} else {
			chunks += int(info.Size())
		}
	}
	fmt.Printf("client runtime: %d bytes after brotli, island chunks: %d\n", runtime, chunks)
	return runtime, chunks, nil
}

func checkClientSize(root string, runtime, chunks int) error {
	baseline, err := readBaseline(filepath.Join(root, lockName))
	if err != nil {
		return err
	}
	current := benchlog.Record{ClientRuntime: runtime, ClientChunks: chunks}
	regressions := benchlog.Compare(benchlog.Record{ClientRuntime: baseline.ClientRuntime, ClientChunks: baseline.ClientChunks}, current)
	for _, regression := range regressions {
		fmt.Println("regression:", regression)
	}
	if len(regressions) > 0 {
		return fmt.Errorf("smoke: the client bundle grew past the performance log; run smoke --record to accept it")
	}
	return nil
}

func recordSizes(root string, worker, runtime, chunks int) error {
	path := filepath.Join(root, lockName)
	baseline, err := readBaseline(path)
	if err != nil {
		return err
	}
	baseline.WorkerSize = worker
	baseline.ClientRuntime = runtime
	baseline.ClientChunks = chunks
	if err := writeBaseline(path, baseline); err != nil {
		return err
	}
	fmt.Println("performance log updated:", path)
	return nil
}

type countingWriter struct {
	n int
}

func (c *countingWriter) Write(p []byte) (int, error) {
	c.n += len(p)
	return len(p), nil
}

func serveAndProbe(project string, keep bool) error {
	port, err := smoke.FreePort()
	if err != nil {
		return err
	}
	logPath := filepath.Join(project, "wrangler.log")
	log, err := os.Create(logPath)
	if err != nil {
		return err
	}
	defer func() { _ = log.Close() }()

	cmd := exec.Command("wrangler", "dev", "--port", fmt.Sprint(port), "--local")
	cmd.Dir = project
	cmd.Stdout = log
	cmd.Stderr = log
	if err := cmd.Start(); err != nil {
		return err
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()

	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	fetch := smokeFetcher()
	if err := smoke.WaitReady(fetch, base, readyAttempts, readyPause); err != nil {
		return withLog(err, logPath, keep)
	}
	if err := smoke.Run(fetch, base); err != nil {
		return withLog(err, logPath, keep)
	}

	fmt.Println("smoke: ok — static assets and the Go worker both answered")
	return nil
}

func withLog(err error, logPath string, keep bool) error {
	if keep {
		return fmt.Errorf("%w (wrangler log: %s)", err, logPath)
	}
	data, readErr := os.ReadFile(logPath)
	if readErr != nil {
		return err
	}
	return fmt.Errorf("%w\nwrangler log:\n%s", err, tail(string(data), 20))
}

func tail(text string, lines int) string {
	var start, seen int
	for i := len(text) - 1; i >= 0; i-- {
		if text[i] == '\n' {
			seen++
			if seen > lines {
				start = i + 1
				break
			}
		}
	}
	return text[start:]
}

func runReference(project string, keep bool) error {
	nativePort, err := smoke.FreePort()
	if err != nil {
		return err
	}
	native := exec.Command(filepath.Join(project, filepath.FromSlash(paths.NativeBinary)))
	native.Dir = project
	native.Env = append(os.Environ(), fmt.Sprintf("ADDR=:%d", nativePort))
	native.Stdout = os.Stderr
	native.Stderr = os.Stderr
	if err := native.Start(); err != nil {
		return err
	}
	defer func() {
		_ = native.Process.Kill()
		_, _ = native.Process.Wait()
	}()

	nativeBase := fmt.Sprintf("http://127.0.0.1:%d", nativePort)
	fetch := smokeFetcher()
	if err := smoke.WaitReady(fetch, nativeBase, readyAttempts, readyPause); err != nil {
		return fmt.Errorf("native binary: %w", err)
	}
	if err := smoke.RunReference(fetch, nativeBase); err != nil {
		return fmt.Errorf("native binary: %w", err)
	}
	if err := smoke.RunCache(fetch, nativeBase); err != nil {
		return fmt.Errorf("native binary: %w", err)
	}
	if err := smoke.RunForm(smokeClient(), nativeBase); err != nil {
		return fmt.Errorf("native binary: %w", err)
	}
	if err := smoke.RunFragment(smokeClient(), nativeBase); err != nil {
		return fmt.Errorf("native binary: %w", err)
	}
	if err := smoke.RunPartial(smokeClient(), nativeBase); err != nil {
		return fmt.Errorf("native binary: %w", err)
	}
	fmt.Println("reference: ok — the native binary answered every check")

	root, err := repoRoot()
	if err != nil {
		return err
	}
	if err := buildProject(root, project, string(build.TargetWorkers)); err != nil {
		return err
	}
	workerPort, err := smoke.FreePort()
	if err != nil {
		return err
	}
	logPath := filepath.Join(project, "wrangler.log")
	log, err := os.Create(logPath)
	if err != nil {
		return err
	}
	defer func() { _ = log.Close() }()

	worker := exec.Command("wrangler", "dev", "--port", fmt.Sprint(workerPort), "--local")
	worker.Dir = project
	worker.Stdout = log
	worker.Stderr = log
	if err := worker.Start(); err != nil {
		return err
	}
	defer func() {
		_ = worker.Process.Kill()
		_, _ = worker.Process.Wait()
	}()

	workerBase := fmt.Sprintf("http://127.0.0.1:%d", workerPort)
	if err := smoke.WaitReady(fetch, workerBase, readyAttempts, readyPause); err != nil {
		return withLog(err, logPath, keep)
	}
	if err := smoke.RunReference(fetch, workerBase); err != nil {
		return withLog(err, logPath, keep)
	}
	fmt.Println("reference: ok — the worker answered every check")

	if err := smoke.CompareAdapters(fetch, fetch, nativeBase, workerBase); err != nil {
		return withLog(err, logPath, keep)
	}
	fmt.Println("reference: ok — both adapters returned the same documents")

	_ = worker.Process.Kill()
	_, _ = worker.Process.Wait()
	_ = native.Process.Kill()
	_, _ = native.Process.Wait()
	return runSubdomain(root, project)
}

func runSubdomain(root, project string) error {
	before, err := smoke.Fingerprint(os.DirFS(project))
	if err != nil {
		return err
	}
	path := filepath.Join(project, paths.Config)
	source, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	switched, err := smoke.SubdomainConfig(string(source))
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(switched), 0o644); err != nil {
		return err
	}
	if err := buildProject(root, project, string(build.TargetNative)); err != nil {
		return err
	}
	after, err := smoke.Fingerprint(os.DirFS(project))
	if err != nil {
		return err
	}
	if changed := smoke.Changed(before, after); len(changed) > 0 {
		return fmt.Errorf("switching to subdomain mode changed %s", strings.Join(changed, ", "))
	}

	port, err := smoke.FreePort()
	if err != nil {
		return err
	}
	server := exec.Command(filepath.Join(project, filepath.FromSlash(paths.NativeBinary)))
	server.Dir = project
	server.Env = append(os.Environ(), fmt.Sprintf("ADDR=:%d", port))
	server.Stdout = os.Stderr
	server.Stderr = os.Stderr
	if err := server.Start(); err != nil {
		return err
	}
	defer func() {
		_ = server.Process.Kill()
		_, _ = server.Process.Wait()
	}()

	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	host := hostFetcher(smokeClient())
	if err := waitHost(host, base); err != nil {
		return err
	}
	if err := smoke.RunSubdomain(host, base); err != nil {
		return fmt.Errorf("subdomain mode: %w", err)
	}
	fmt.Println("reference: ok — the same templates served both routing modes")
	return nil
}

func waitHost(fetch smoke.HostFetcher, base string) error {
	return smoke.WaitReady(func(url string) (smoke.Response, error) {
		return fetch(smoke.DefaultHost, url)
	}, base, readyAttempts, readyPause)
}

func hostFetcher(client *http.Client) smoke.HostFetcher {
	return func(host, url string) (smoke.Response, error) {
		request, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return smoke.Response{}, err
		}
		request.Host = host
		response, err := client.Do(request)
		if err != nil {
			return smoke.Response{}, err
		}
		defer func() { _ = response.Body.Close() }()
		body, err := io.ReadAll(response.Body)
		if err != nil {
			return smoke.Response{}, err
		}
		return smoke.Response{
			Status:      response.StatusCode,
			Body:        string(body),
			ContentType: response.Header.Get("Content-Type"),
			Headers:     response.Header,
		}, nil
	}
}

func probeDev(root, project string) error {
	port, err := smoke.FreePort()
	if err != nil {
		return err
	}
	binary := filepath.Join(filepath.Dir(project), "rill")
	compile := exec.Command("go", "build", "-o", binary, "./cmd/rill")
	compile.Dir = root
	compile.Stderr = os.Stderr
	if err := compile.Run(); err != nil {
		return fmt.Errorf("building rill: %w", err)
	}
	command := exec.Command(binary, "dev",
		"--dir", project, "--addr", fmt.Sprintf(":%d", port))
	command.Dir = root
	command.Stdout = os.Stderr
	command.Stderr = os.Stderr
	if err := command.Start(); err != nil {
		return err
	}
	defer func() {
		_ = command.Process.Signal(os.Interrupt)
		_, _ = command.Process.Wait()
	}()

	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	fetch := smokeFetcher()
	if err := smoke.WaitReady(fetch, base, readyAttempts, readyPause); err != nil {
		return fmt.Errorf("rill dev: %w", err)
	}
	if err := smoke.RunDev(fetch, base); err != nil {
		return fmt.Errorf("rill dev: %w", err)
	}

	page := filepath.Join(project, "app", "page.rill")
	source, err := os.ReadFile(page)
	if err != nil {
		return err
	}
	if err := os.WriteFile(page, append(source, []byte("\n"+smoke.DevMarker+"\n")...), 0o644); err != nil {
		return err
	}
	if err := waitFor(fetch, base, func() error { return smoke.VerifyRebuild(fetch, base) }); err != nil {
		return fmt.Errorf("rill dev: %w", err)
	}

	if err := os.WriteFile(page, append(source, []byte("\n<p>{{ \n")...), 0o644); err != nil {
		return err
	}
	if err := waitFor(fetch, base, func() error {
		response, err := fetch(base + "/")
		if err != nil {
			return err
		}
		if !smoke.Overlaid(response) {
			return errors.New("a broken template should show the overlay")
		}
		return nil
	}); err != nil {
		return fmt.Errorf("rill dev: %w", err)
	}

	if err := os.WriteFile(page, source, 0o644); err != nil {
		return err
	}
	if err := waitFor(fetch, base, func() error { return smoke.RunDev(fetch, base) }); err != nil {
		return fmt.Errorf("rill dev did not recover: %w", err)
	}
	fmt.Println("smoke: ok — rill dev served the application, rebuilt it and survived a broken template")
	return nil
}

func waitFor(_ smoke.Fetcher, _ string, check func() error) error {
	var last error
	for range readyAttempts {
		if last = check(); last == nil {
			return nil
		}
		time.Sleep(readyPause)
	}
	return last
}
