import { defineConfig } from "@vscode/test-cli";

export default defineConfig({
	label: "integration",
	files: "out/test/integration/**/*.test.js",
	workspaceFolder: "./test/fixtures/workspace",
	launchArgs: ["--disable-extensions"],
	mocha: { ui: "tdd", timeout: 30000 },
});
