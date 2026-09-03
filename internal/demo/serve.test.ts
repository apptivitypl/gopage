import { describe, expect, it } from "vitest";

import { candidates, contentType, handler, requestOf, workerFirst } from "./serve.mjs";

function message(url: string, method = "GET", headers: Record<string, string> = {}) {
	const raw: string[] = [];
	for (const [name, value] of Object.entries(headers)) {
		raw.push(name, value);
	}
	return { url, method, headers: { host: "localhost:3000", ...headers }, rawHeaders: raw };
}

function sink() {
	const written: Buffer[] = [];
	return {
		status: 0,
		headers: {} as Record<string, string | number>,
		body: "",
		writeHead(status: number, headers: Record<string, string | number>) {
			this.status = status;
			this.headers = headers ?? {};
		},
		write(chunk: Uint8Array) {
			written.push(Buffer.from(chunk));
		},
		end(chunk?: Uint8Array) {
			if (chunk) {
				written.push(Buffer.from(chunk));
			}
			this.body = Buffer.concat(written).toString();
		},
	};
}

describe("contentType", () => {
	it("names the types a rill build emits", () => {
		expect(contentType("app.css")).toBe("text/css; charset=utf-8");
		expect(contentType("island.JS.js")).toBe("text/javascript; charset=utf-8");
		expect(contentType("sky.avif")).toBe("image/avif");
		expect(contentType("jetbrains.WOFF2")).toBe("font/woff2");
	});

	it("falls back to bytes when it cannot tell", () => {
		expect(contentType("LICENSE")).toBe("application/octet-stream");
		expect(contentType("thing.unknown")).toBe("application/octet-stream");
	});
});

describe("workerFirst", () => {
	it("matches the patterns a workers build writes", () => {
		const patterns = ["/", "/api/*", "/listings/*"];
		expect(workerFirst(patterns, "/")).toBe(true);
		expect(workerFirst(patterns, "/api/stories")).toBe(true);
		expect(workerFirst(patterns, "/listings/9")).toBe(true);
		expect(workerFirst(patterns, "/about")).toBe(false);
	});

	it("does not let a pattern leak into a regular expression", () => {
		expect(workerFirst(["/a.b"], "/axb")).toBe(false);
	});
});

describe("candidates", () => {
	it("asks for index.html where a page would be", () => {
		expect(candidates("/")).toEqual(["index.html"]);
		expect(candidates("/about")).toEqual(["about", "about/index.html"]);
		expect(candidates("/assets/app.css")).toEqual(["assets/app.css"]);
	});

	it("refuses to climb out of the asset folder", () => {
		expect(candidates("/../secret")).toEqual([]);
		expect(candidates("/%2e%2e/secret")).toEqual([]);
		expect(candidates("/%ZZ")).toEqual([]);
	});
});

describe("requestOf", () => {
	it("carries the headers over", () => {
		const request = requestOf(message("/x", "GET", { "x-test": "1" }), 3000);
		expect(request.url).toBe("http://localhost:3000/x");
		expect(request.headers.get("x-test")).toBe("1");
	});
});

describe("handler", () => {
	const page = new TextEncoder().encode("<h1>about</h1>");

	it("serves an asset without waking the worker", async () => {
		let asked = false;
		const response = sink();
		await handler({
			patterns: ["/api/*"],
			asset: async (name: string) => (name === "about/index.html" ? page : undefined),
			worker: async () => {
				asked = true;
				return new Response("no", { status: 500 });
			},
			port: 3000,
		})(message("/about"), response);
		expect(asked).toBe(false);
		expect(response.status).toBe(200);
		expect(response.headers["content-type"]).toBe("text/html; charset=utf-8");
		expect(response.body).toBe("<h1>about</h1>");
	});

	it("leaves the body out of a HEAD", async () => {
		const response = sink();
		await handler({
			patterns: [],
			asset: async () => page,
			worker: async () => new Response("no"),
			port: 3000,
		})(message("/about", "HEAD"), response);
		expect(response.body).toBe("");
		expect(response.headers["content-length"]).toBe(page.byteLength);
	});

	it("hands a worker-first path straight to the worker", async () => {
		const response = sink();
		await handler({
			patterns: ["/api/*"],
			asset: async () => page,
			worker: async () => new Response(`{"ok":true}`, { headers: { "content-type": "application/json" } }),
			port: 3000,
		})(message("/api/health"), response);
		expect(response.body).toBe(`{"ok":true}`);
		expect(response.headers["content-type"]).toBe("application/json");
	});

	it("falls through to the worker when no asset matches", async () => {
		const response = sink();
		await handler({
			patterns: [],
			asset: async () => undefined,
			worker: async () => new Response("not found", { status: 404 }),
			port: 3000,
		})(message("/missing"), response);
		expect(response.status).toBe(404);
		expect(response.body).toBe("not found");
	});

	it("answers an empty body without hanging", async () => {
		const response = sink();
		await handler({
			patterns: [],
			asset: async () => undefined,
			worker: async () => new Response(null, { status: 204 }),
			port: 3000,
		})(message("/gone"), response);
		expect(response.status).toBe(204);
		expect(response.body).toBe("");
	});

	it("reports a worker that threw instead of hanging up", async () => {
		const response = sink();
		await handler({
			patterns: [],
			asset: async () => undefined,
			worker: async () => {
				throw new Error("wasm trap");
			},
			port: 3000,
		})(message("/boom"), response);
		expect(response.status).toBe(500);
		expect(response.body).toContain("wasm trap");
	});
});
