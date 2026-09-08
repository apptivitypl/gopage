package compile

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/apptivitypl/gopage/internal/diag"
)

func imaged(t *testing.T, page, settings string) string {
	t.Helper()
	files := app(map[string]string{"app/page.gopage": page})
	files["gopage.jsonc"] = &fstest.MapFile{Data: []byte(settings)}
	var bag diag.Bag
	result, err := Compile(files, &bag)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if bag.HasErrors() {
		t.Fatalf("diagnostics = %+v", bag.Items())
	}
	return string(result.Manifest.Plans[result.Manifest.Routes[0].Plan].Blob)
}

const optimising = `{"images": {"mode": "on", "widths": [320, 640, 1280]}}`

func TestAnImageIsServedThroughTheEndpoint(t *testing.T) {
	blob := imaged(t, `<Image src="/hero.jpg" width="960" height="540" alt="hero" />`, optimising)
	for _, want := range []string{
		`src="/_gopage/image?src=%2Fhero.jpg&amp;w=960"`,
		"/_gopage/image?src=%2Fhero.jpg&amp;w=320 320w",
		"/_gopage/image?src=%2Fhero.jpg&amp;w=640 640w",
		"/_gopage/image?src=%2Fhero.jpg&amp;w=960 960w",
	} {
		if !strings.Contains(blob, want) {
			t.Errorf("blob = %q, want %q", blob, want)
		}
	}
	if strings.Contains(blob, "w=1280") {
		t.Errorf("blob = %q, want no width above the declared one", blob)
	}
}

func TestOptimisationIsOffUntilItIsAskedFor(t *testing.T) {
	blob := imaged(t, `<Image src="/hero.jpg" width="960" height="540" alt="hero" />`, "{}")
	if strings.Contains(blob, "_gopage/image") {
		t.Errorf("blob = %q, want the source untouched", blob)
	}
}

func TestAnImageMayOptOut(t *testing.T) {
	blob := imaged(t, `<Image src="/hero.jpg" width="960" height="540" alt="hero" plain />`, optimising)
	if strings.Contains(blob, "_gopage/image") {
		t.Errorf("blob = %q", blob)
	}
	if !strings.Contains(blob, `src="/hero.jpg"`) {
		t.Errorf("blob = %q", blob)
	}
}

func TestSizesRideAlong(t *testing.T) {
	blob := imaged(t, `<Image src="/hero.jpg" width="960" height="540" alt="hero" sizes="(max-width: 600px) 100vw, 960px" />`, optimising)
	if !strings.Contains(blob, `sizes="(max-width: 600px) 100vw, 960px"`) {
		t.Errorf("blob = %q", blob)
	}
}

func TestARelativeSourceThatIsNotRootedIsLeftAlone(t *testing.T) {
	blob := imaged(t, `<Image src="hero.jpg" width="960" height="540" alt="hero" />`, optimising)
	if strings.Contains(blob, "_gopage/image") {
		t.Errorf("blob = %q", blob)
	}
}

func TestIconsAndTheManifestAreLinked(t *testing.T) {
	files := app(map[string]string{"app/page.gopage": "{% assets %}"})
	for _, name := range []string{"favicon.ico", "icon.svg", "apple-icon.png", "manifest.json"} {
		files["public/"+name] = &fstest.MapFile{Data: []byte("x")}
	}
	var bag diag.Bag
	result, err := Compile(files, &bag)
	if err != nil || bag.HasErrors() {
		t.Fatalf("Compile: %v, %+v", err, bag.Items())
	}
	blob := string(result.Manifest.Plans[result.Manifest.Routes[0].Plan].Blob)
	for _, want := range []string{
		`<link rel="icon" href="/favicon.ico" sizes="any">`,
		`<link rel="icon" href="/icon.svg" type="image/svg+xml">`,
		`<link rel="apple-touch-icon" href="/apple-icon.png">`,
		`<link rel="manifest" href="/manifest.json">`,
	} {
		if !strings.Contains(blob, want) {
			t.Errorf("blob = %q, want %q", blob, want)
		}
	}
}

func TestAProjectWithoutIconsLinksNone(t *testing.T) {
	files := app(map[string]string{"app/page.gopage": "{% assets %}"})
	var bag diag.Bag
	result, err := Compile(files, &bag)
	if err != nil || bag.HasErrors() {
		t.Fatalf("Compile: %v, %+v", err, bag.Items())
	}
	if strings.Contains(string(result.Manifest.Plans[result.Manifest.Routes[0].Plan].Blob), "rel=\"icon\"") {
		t.Error("nothing to link")
	}
}
