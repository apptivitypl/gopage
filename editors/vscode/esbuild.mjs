import { context } from "esbuild";

const production = process.argv.includes("--production");
const watch = process.argv.includes("--watch");

const builder = await context({
	entryPoints: ["src/extension.ts"],
	bundle: true,
	format: "cjs",
	platform: "node",
	target: "node20",
	outfile: "dist/extension.js",
	external: ["vscode"],
	minify: production,
	sourcemap: !production,
	sourcesContent: false,
	logLevel: "warning",
});

if (watch) {
	await builder.watch();
} else {
	await builder.rebuild();
	await builder.dispose();
}
