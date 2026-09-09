import { defineConfig } from "vitest/config";

export default defineConfig({
	test: {
		include: ["src/**/*.test.ts"],
		coverage: {
			provider: "v8",
			include: ["src/binary.ts"],
			thresholds: { statements: 90, branches: 85, functions: 90, lines: 90 },
			reporter: ["text-summary"],
		},
	},
});
