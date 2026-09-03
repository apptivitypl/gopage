import { defineConfig } from "vitest/config";

export default defineConfig({
	test: {
		environment: "happy-dom",
		include: ["internal/**/*.test.ts"],
		coverage: {
			provider: "v8",
			include: ["internal/bundle/runtime.ts", "internal/demo/serve.mjs"],
			thresholds: { statements: 90, branches: 85, functions: 90, lines: 90 },
			reporter: ["text-summary"],
		},
	},
});
