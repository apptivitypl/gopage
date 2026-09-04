package paths

import "runtime"

const (
	Config   = "gopage.jsonc"
	Wrangler = "wrangler.jsonc"
)

const (
	AppDir        = "app"
	ComponentsDir = "components"
	LocalesDir    = "locales"
	PublicDir     = "public"
	StylesDir     = "styles"
)

const (
	GenRoot    = "internal/gen"
	Manifest   = "internal/gen/manifest.bin"
	GenConfig  = "internal/gen/config.json"
	PropsDir   = "internal/gen/props"
	PropsTypes = "internal/gen/props.d.ts"
	GenStyles  = "internal/gen/styles"
	GenPublic  = "internal/gen/public"
	GenBundles = "internal/gen/bundles"
)

const (
	CacheRoot = ".gopage"
	CacheDir  = ".gopage/cache"
	Inventory = ".gopage/cache/tailwind-inventory.txt"
)

const (
	DistRoot     = "dist"
	AssetsDir    = "dist/assets"
	Redirects    = "dist/assets/_redirects"
	Headers      = "dist/assets/_headers"
	WorkerDir    = "dist/worker"
	WorkerBinary = "dist/worker/app.wasm"
	NativeBinary = "dist/server"
	DemoDir      = "dist/demo"
	DemoBinary   = "dist/demo/app.wasm"
	DemoAssets   = "dist/demo/assets"
)

const (
	ServerMain = "./cmd/server"
	WorkerMain = "./cmd/worker"
)

func ServerBinary(goos string) string {
	if goos == "windows" {
		return NativeBinary + ".exe"
	}
	return NativeBinary
}

func Server() string {
	return ServerBinary(runtime.GOOS)
}

func Generated() []string {
	return []string{GenRoot, DistRoot, CacheRoot}
}
