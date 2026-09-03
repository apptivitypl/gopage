#!/usr/bin/env node
import { createServer } from "node:http";
import { readFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { dirname, join, sep } from "node:path";

import { handler } from "./serve.mjs";
import worker from "./worker.mjs";

const here = dirname(fileURLToPath(import.meta.url));
const meta = JSON.parse(await readFile(join(here, "demo.json"), "utf8"));
const assets = join(here, "assets");
const port = Number(process.env.PORT ?? 3000);

async function asset(candidate) {
	const path = join(assets, candidate.split("/").join(sep));
	if (!path.startsWith(assets + sep)) {
		return undefined;
	}
	try {
		return await readFile(path);
	} catch {
		return undefined;
	}
}

const server = createServer(handler({
	patterns: meta.workerFirst,
	asset,
	worker: (request) => worker.fetch(request),
	port,
}));

server.listen(port, () => {
	console.log(`${meta.name} is on http://localhost:${port}`);
});
