const types = new Map([
	[".avif", "image/avif"],
	[".css", "text/css; charset=utf-8"],
	[".html", "text/html; charset=utf-8"],
	[".ico", "image/x-icon"],
	[".jpg", "image/jpeg"],
	[".js", "text/javascript; charset=utf-8"],
	[".json", "application/json; charset=utf-8"],
	[".map", "application/json; charset=utf-8"],
	[".mjs", "text/javascript; charset=utf-8"],
	[".png", "image/png"],
	[".svg", "image/svg+xml"],
	[".txt", "text/plain; charset=utf-8"],
	[".wasm", "application/wasm"],
	[".webp", "image/webp"],
	[".woff2", "font/woff2"],
	[".xml", "application/xml; charset=utf-8"],
]);

export function contentType(path) {
	const dot = path.lastIndexOf(".");
	if (dot < 0) {
		return "application/octet-stream";
	}
	return types.get(path.slice(dot).toLowerCase()) ?? "application/octet-stream";
}

export function workerFirst(patterns, pathname) {
	return patterns.some((pattern) => {
		const escaped = pattern.replace(/[.+?^${}()|[\]\\]/g, "\\$&");
		return new RegExp("^" + escaped.replace(/\*/g, ".*") + "$").test(pathname);
	});
}

export function candidates(pathname) {
	let decoded;
	try {
		decoded = decodeURIComponent(pathname);
	} catch {
		return [];
	}
	if (decoded.includes("\0") || decoded.split("/").includes("..")) {
		return [];
	}
	const trimmed = decoded.replace(/^\/+/, "").replace(/\/+$/, "");
	if (trimmed === "") {
		return ["index.html"];
	}
	if (trimmed.includes(".")) {
		return [trimmed];
	}
	return [trimmed, trimmed + "/index.html"];
}

export function requestOf(message, port) {
	const host = message.headers.host ?? `localhost:${port}`;
	const url = new URL(message.url, `http://${host}`);
	const headers = new Headers();
	for (let i = 0; i < message.rawHeaders.length; i += 2) {
		headers.append(message.rawHeaders[i], message.rawHeaders[i + 1]);
	}
	const method = message.method ?? "GET";
	const body = method === "GET" || method === "HEAD" ? undefined : message;
	return new Request(url, { method, headers, body, duplex: body ? "half" : undefined });
}

export function handler({ patterns, asset, worker, port }) {
	return async (message, response) => {
		try {
			const request = requestOf(message, port);
			const pathname = new URL(request.url).pathname;
			if (!workerFirst(patterns, pathname)) {
				for (const candidate of candidates(pathname)) {
					const found = await asset(candidate);
					if (found) {
						response.writeHead(200, {
							"content-type": contentType(candidate),
							"content-length": found.byteLength,
						});
						response.end(message.method === "HEAD" ? undefined : found);
						return;
					}
				}
			}
			await reply(response, await worker(request), message.method === "HEAD");
		} catch (error) {
			response.writeHead(500, { "content-type": "text/plain; charset=utf-8" });
			response.end(String(error?.stack ?? error) + "\n");
		}
	};
}

export async function reply(response, result, headOnly) {
	const headers = {};
	result.headers.forEach((value, name) => {
		headers[name] = value;
	});
	response.writeHead(result.status, headers);
	if (headOnly || !result.body) {
		response.end();
		return;
	}
	for await (const chunk of result.body) {
		response.write(chunk);
	}
	response.end();
}
