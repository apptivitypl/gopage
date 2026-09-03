#!/usr/bin/env node
"use strict";

const { spawnSync } = require("node:child_process");

let entry;
try {
	entry = require.resolve("@apptivitypl/rill/bin/rill.js");
} catch {
	process.stderr.write("create-rill: @apptivitypl/rill is missing; run it again with pnpm create rill\n");
	process.exit(1);
}

const result = spawnSync(process.execPath, [entry, "new", ...process.argv.slice(2)], { stdio: "inherit" });
if (result.error) {
	process.stderr.write(`create-rill: ${result.error.message}\n`);
	process.exit(1);
}
process.exit(result.status === null ? 1 : result.status);
