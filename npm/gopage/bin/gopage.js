#!/usr/bin/env node
"use strict";

const { spawnSync } = require("node:child_process");

const systems = { darwin: "darwin", linux: "linux", win32: "win32" };
const architectures = { x64: "x64", arm64: "arm64" };

function fail(message) {
	process.stderr.write(`@apptivitypl/gopage: ${message}\n`);
	process.exit(1);
}

function binary() {
	const system = systems[process.platform];
	const architecture = architectures[process.arch];
	if (!system || !architecture) {
		fail(`there is no gopage build for ${process.platform} ${process.arch}`);
	}
	const name = `@apptivitypl/gopage-${system}-${architecture}`;
	const file = system === "win32" ? "gopage.exe" : "gopage";
	try {
		return require.resolve(`${name}/bin/${file}`);
	} catch {
		fail(`${name} is missing; install it, or install @apptivitypl/gopage again without --no-optional`);
	}
}

const result = spawnSync(binary(), process.argv.slice(2), { stdio: "inherit" });
if (result.error) {
	fail(result.error.message);
}
process.exit(result.status === null ? 1 : result.status);
