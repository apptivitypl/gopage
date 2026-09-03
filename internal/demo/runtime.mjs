const location = new URL("./app.wasm", import.meta.url);

let pending;

function isNode() {
	return typeof process !== "undefined" && process.versions?.node !== undefined;
}

async function compile() {
	if (isNode()) {
		const { readFile } = await import("node:fs/promises");
		return WebAssembly.compile(await readFile(location));
	}
	return WebAssembly.compileStreaming(fetch(location));
}

export async function loadModule() {
	pending ??= compile();
	return pending;
}

export function createRuntimeContext({ binding }) {
	return { binding };
}
