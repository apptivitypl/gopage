import { beforeEach, describe, expect, it, vi } from "vitest";
import { hydrate, navigation, pull, register, release, slots, start } from "./runtime";

type Props = Record<string, unknown>;

function island(name: string, strategy: string, props: Props, media?: string): HTMLElement {
	const element = document.createElement("gopage-island");
	element.setAttribute("name", name);
	element.setAttribute("strategy", strategy);
	if (media) {
		element.setAttribute("media", media);
	}
	const script = document.createElement("script");
	script.type = "application/json";
	script.textContent = JSON.stringify(props);
	element.append(script);
	const body = document.createElement("div");
	body.className = "body";
	element.append(body);
	document.body.append(element);
	return element;
}

function observers(): { fire: () => void; observed: Element[] } {
	const observed: Element[] = [];
	let callback: IntersectionObserverCallback = () => {};
	vi.stubGlobal(
		"IntersectionObserver",
		class {
			constructor(cb: IntersectionObserverCallback) {
				callback = cb;
			}
			observe(element: Element) {
				observed.push(element);
			}
			disconnect() {}
		},
	);
	return {
		observed,
		fire: () => callback([{ isIntersecting: true } as IntersectionObserverEntry], {} as IntersectionObserver),
	};
}

beforeEach(() => {
	document.body.innerHTML = "";
	vi.unstubAllGlobals();
});

describe("hydrate", () => {
	it("mounts an island with the props from its script", async () => {
		const seen: Props[] = [];
		register("Counter", async () => ({ mount: (_el, props) => void seen.push(props) }));
		island("Counter", "load", { Start: 4 });

		hydrate();
		await vi.waitFor(() => expect(seen).toHaveLength(1));
		expect(seen[0]).toEqual({ Start: 4 });
	});

	it("passes the island element to mount", async () => {
		let target: HTMLElement | null = null;
		register("Target", async () => ({ mount: (el) => void (target = el) }));
		const element = island("Target", "load", {});

		hydrate();
		await vi.waitFor(() => expect(target).not.toBeNull());
		expect(target).toBe(element);
	});

	it("mounts an island only once", async () => {
		const mount = vi.fn();
		register("Once", async () => ({ mount }));
		island("Once", "load", {});

		hydrate();
		hydrate();
		await vi.waitFor(() => expect(mount).toHaveBeenCalledTimes(1));
	});

	it("leaves an island whose module was never registered alone", async () => {
		island("Missing", "load", {});
		hydrate();
		await new Promise((resolve) => setTimeout(resolve, 10));
		expect(document.querySelector("gopage-island")?.isConnected).toBe(true);
	});

	it("ignores an island without a name", () => {
		const element = document.createElement("gopage-island");
		document.body.append(element);
		expect(() => hydrate()).not.toThrow();
	});

	it("treats malformed props as empty", async () => {
		const seen: Props[] = [];
		register("Broken", async () => ({ mount: (_el, props) => void seen.push(props) }));
		const element = island("Broken", "load", {});
		element.querySelector("script")!.textContent = "{not json";

		hydrate();
		await vi.waitFor(() => expect(seen).toHaveLength(1));
		expect(seen[0]).toEqual({});
	});

	it("treats a missing props script as empty", async () => {
		const seen: Props[] = [];
		register("Bare", async () => ({ mount: (_el, props) => void seen.push(props) }));
		const element = island("Bare", "load", {});
		element.querySelector("script")!.remove();

		hydrate();
		await vi.waitFor(() => expect(seen).toHaveLength(1));
		expect(seen[0]).toEqual({});
	});
});

describe("strategies", () => {
	it("waits for the island to become visible", async () => {
		const mount = vi.fn();
		const { fire, observed } = observers();
		register("Visible", async () => ({ mount }));
		const element = island("Visible", "visible", {});

		hydrate();
		expect(mount).not.toHaveBeenCalled();
		expect(observed[0]).toBe(element.querySelector(".body"));

		fire();
		await vi.waitFor(() => expect(mount).toHaveBeenCalledTimes(1));
	});

	it("mounts at once when the browser has no observer", async () => {
		const mount = vi.fn();
		vi.stubGlobal("IntersectionObserver", undefined);
		register("NoObserver", async () => ({ mount }));
		island("NoObserver", "visible", {});

		hydrate();
		await vi.waitFor(() => expect(mount).toHaveBeenCalledTimes(1));
	});

	it("waits for an idle callback", async () => {
		const mount = vi.fn();
		const idle = vi.fn((run: () => void) => run());
		vi.stubGlobal("requestIdleCallback", idle);
		register("Idle", async () => ({ mount }));
		island("Idle", "idle", {});

		hydrate();
		await vi.waitFor(() => expect(mount).toHaveBeenCalledTimes(1));
		expect(idle).toHaveBeenCalled();
	});

	it("falls back to a timeout without an idle callback", async () => {
		const mount = vi.fn();
		vi.stubGlobal("requestIdleCallback", undefined);
		register("Timeout", async () => ({ mount }));
		island("Timeout", "idle", {});

		hydrate();
		await vi.waitFor(() => expect(mount).toHaveBeenCalledTimes(1));
	});

	it("mounts at once when the media query already matches", async () => {
		const mount = vi.fn();
		vi.stubGlobal("matchMedia", () => ({ matches: true, addEventListener: vi.fn(), removeEventListener: vi.fn() }));
		register("Wide", async () => ({ mount }));
		island("Wide", "media", {}, "(min-width: 40rem)");

		hydrate();
		await vi.waitFor(() => expect(mount).toHaveBeenCalledTimes(1));
	});

	it("waits for the media query to start matching", async () => {
		const mount = vi.fn();
		const list = {
			matches: false,
			listeners: [] as (() => void)[],
			addEventListener(_: string, fn: () => void) {
				this.listeners.push(fn);
			},
			removeEventListener() {},
		};
		vi.stubGlobal("matchMedia", () => list);
		register("Later", async () => ({ mount }));
		island("Later", "media", {}, "(min-width: 40rem)");

		hydrate();
		expect(mount).not.toHaveBeenCalled();

		list.matches = true;
		for (const fn of list.listeners) {
			fn();
		}
		await vi.waitFor(() => expect(mount).toHaveBeenCalledTimes(1));
	});

	it("mounts at once without a media query", async () => {
		const mount = vi.fn();
		register("NoQuery", async () => ({ mount }));
		island("NoQuery", "media", {});

		hydrate();
		await vi.waitFor(() => expect(mount).toHaveBeenCalledTimes(1));
	});

	it("treats an unknown strategy as load", async () => {
		const mount = vi.fn();
		register("Odd", async () => ({ mount }));
		island("Odd", "whenever", {});

		hydrate();
		await vi.waitFor(() => expect(mount).toHaveBeenCalledTimes(1));
	});
});

describe("release", () => {
	it("runs the cleanup a mount returned", async () => {
		const stop = vi.fn();
		register("Cleanup", async () => ({ mount: () => stop }));
		island("Cleanup", "load", {});

		hydrate();
		await vi.waitFor(() => expect(document.querySelector("gopage-island")).not.toBeNull());
		await new Promise((resolve) => setTimeout(resolve, 5));

		release();
		expect(stop).toHaveBeenCalledTimes(1);
	});

	it("survives a mount that returns nothing", async () => {
		register("Silent", async () => ({ mount: () => undefined }));
		island("Silent", "load", {});

		hydrate();
		await new Promise((resolve) => setTimeout(resolve, 5));
		expect(() => release()).not.toThrow();
	});

	it("lets an island mount again after it was released", async () => {
		const mount = vi.fn();
		register("Again", async () => ({ mount }));
		island("Again", "load", {});

		hydrate();
		await vi.waitFor(() => expect(mount).toHaveBeenCalledTimes(1));
		release();
		hydrate();
		await vi.waitFor(() => expect(mount).toHaveBeenCalledTimes(2));
	});
});

describe("start", () => {
	it("hydrates straight away when the document is ready", async () => {
		const mount = vi.fn();
		register("Ready", async () => ({ mount }));
		island("Ready", "load", {});

		start();
		await vi.waitFor(() => expect(mount).toHaveBeenCalledTimes(1));
	});

	it("waits for the document when it is still loading", async () => {
		const mount = vi.fn();
		register("Loading", async () => ({ mount }));
		island("Loading", "load", {});
		vi.spyOn(document, "readyState", "get").mockReturnValue("loading");

		start();
		expect(mount).not.toHaveBeenCalled();

		document.dispatchEvent(new Event("DOMContentLoaded"));
		await vi.waitFor(() => expect(mount).toHaveBeenCalledTimes(1));
	});
});

describe("navigation", () => {
	function anchor(href: string, attrs: Record<string, string> = {}): HTMLAnchorElement {
		const link = document.createElement("a");
		link.href = href;
		for (const [name, value] of Object.entries(attrs)) {
			link.setAttribute(name, value);
		}
		document.body.append(link);
		return link;
	}

	function shell(): void {
		document.body.innerHTML = "";
		const nav = document.createElement("nav");
		nav.textContent = "menu";
		document.body.append(nav);
		document.body.append(document.createComment("gopage:o0"));
		const page = document.createElement("h1");
		page.textContent = "home";
		document.body.append(page);
		document.body.append(document.createComment("/gopage:o0"));
	}

	function respond(body: string, headers: Record<string, string> = {}): void {
		const all = { "content-type": "text/vnd.gopage-partial", ...headers };
		vi.stubGlobal(
			"fetch",
			vi.fn(async () => ({
				ok: true,
				url: "",
				headers: { get: (name: string) => all[name] ?? null },
				text: async () => body,
			})),
		);
	}

	function click(link: HTMLAnchorElement): void {
		link.dispatchEvent(new MouseEvent("click", { bubbles: true, cancelable: true, button: 0 }));
	}

	beforeEach(() => {
		vi.restoreAllMocks();
		history.replaceState(null, "", "/");
		document.title = "";
	});

	it("swaps the outlet and keeps the shared layout", async () => {
		shell();
		const link = anchor("/docs");
		respond("<h1>docs</h1>", { "GOPAGE-Level": "1", "GOPAGE-Title": "docs" });

		navigation();
		click(link);
		await vi.waitFor(() => expect(document.querySelector("h1")?.textContent).toBe("docs"));

		expect(document.querySelector("nav")?.textContent).toBe("menu");
		expect(document.title).toBe("docs");
		expect(location.pathname).toBe("/docs");
	});

	it("clears a stale title when the next page has none", async () => {
		shell();
		document.title = "docs";
		const link = anchor("/other");
		respond("<h1>other</h1>", { "GOPAGE-Level": "1", "GOPAGE-Title": "" });

		navigation();
		click(link);
		await vi.waitFor(() => expect(document.querySelector("h1")?.textContent).toBe("other"));
		expect(document.title).toBe("");
	});

	it("decodes an escaped title", async () => {
		shell();
		const link = anchor("/pl");
		respond("<h1>pl</h1>", { "GOPAGE-Level": "1", "GOPAGE-Title": "%C5%BC%C3%B3%C5%82w+%26+co" });

		navigation();
		click(link);
		await vi.waitFor(() => expect(document.title).toBe("żółw & co"));
	});

	it("tells the server where it is coming from", async () => {
		shell();
		history.replaceState(null, "", "/start");
		const link = anchor("/docs");
		respond("<h1>docs</h1>", { "GOPAGE-Level": "1" });

		navigation();
		click(link);
		await vi.waitFor(() => expect(fetch).toHaveBeenCalled());
		const [, init] = (fetch as unknown as { mock: { calls: [string, RequestInit][] } }).mock.calls[0];
		expect((init.headers as Record<string, string>)["GOPAGE-Partial"]).toBe("/start");
	});

	it("leaves the page alone when nothing is shared", async () => {
		shell();
		const link = anchor("/elsewhere");
		respond("<h1>replaced</h1>", { "GOPAGE-Level": "0" });

		navigation();
		click(link);
		await vi.waitFor(() => expect(fetch).toHaveBeenCalled());
		await new Promise((resolve) => setTimeout(resolve, 20));
		expect(document.querySelector("h1")?.textContent).toBe("home");
	});

	it("hydrates islands that arrive with the new page", async () => {
		shell();
		const mount = vi.fn();
		register("Fresh", async () => ({ mount }));
		const link = anchor("/docs");
		respond(
			'<gopage-island name="Fresh" strategy="load"><script type="application/json">{}</script><div></div></gopage-island>',
			{ "GOPAGE-Level": "1" },
		);

		navigation();
		click(link);
		await vi.waitFor(() => expect(mount).toHaveBeenCalledTimes(1));
	});

	it("releases islands that leave with the old page", async () => {
		shell();
		const stop = vi.fn();
		register("Leaving", async () => ({ mount: () => stop }));
		const island = document.createElement("gopage-island");
		island.setAttribute("name", "Leaving");
		island.setAttribute("strategy", "load");
		island.append(document.createElement("div"));
		document.querySelector("h1")!.append(island);

		hydrate();
		await new Promise((resolve) => setTimeout(resolve, 5));

		const link = anchor("/docs");
		respond("<h1>docs</h1>", { "GOPAGE-Level": "1" });
		navigation();
		click(link);
		await vi.waitFor(() => expect(stop).toHaveBeenCalledTimes(1));
	});

	const skipped: Record<string, [string, Record<string, string>]> = {
		"another origin": ["https://other.test/x", {}],
		"a download": ["/download", { download: "" }],
		"an opted out link": ["/opt-out", { "data-gopage-nav": "off" }],
		"another target": ["/target", { target: "_blank" }],
	};

	for (const [name, [href, attrs]] of Object.entries(skipped)) {
		it(`leaves ${name} to the browser`, () => {
			shell();
			respond("<h1>x</h1>", { "GOPAGE-Level": "1" });
			navigation();
			click(anchor(href, attrs));
			expect(fetch).not.toHaveBeenCalled();
		});
	}

	it("leaves a link to the current page alone", () => {
		shell();
		respond("<h1>x</h1>", { "GOPAGE-Level": "1" });
		navigation();
		click(anchor(location.pathname));
		expect(fetch).not.toHaveBeenCalled();
	});

	it("leaves the page alone when the answer is not a partial", async () => {
		shell();
		const link = anchor("/api/health");
		vi.stubGlobal(
			"fetch",
			vi.fn(async () => ({
				ok: true,
				url: "",
				headers: { get: (name: string) => (name === "content-type" ? "application/json" : null) },
				text: async () => `{"status":"ok"}`,
			})),
		);

		navigation();
		click(link);
		await vi.waitFor(() => expect(fetch).toHaveBeenCalled());
		await new Promise((resolve) => setTimeout(resolve, 20));
		expect(document.querySelector("h1")?.textContent).toBe("home");
		expect(document.body.textContent).not.toContain("status");
	});

	it("leaves modified clicks to the browser", () => {
		shell();
		const link = anchor("/docs");
		respond("<h1>docs</h1>", { "GOPAGE-Level": "1" });
		navigation();

		link.dispatchEvent(new MouseEvent("click", { bubbles: true, cancelable: true, button: 0, metaKey: true }));
		link.dispatchEvent(new MouseEvent("click", { bubbles: true, cancelable: true, button: 1 }));
		expect(fetch).not.toHaveBeenCalled();
	});

	it("marks the document busy while it waits", async () => {
		shell();
		const link = anchor("/docs");
		let release: (value: unknown) => void = () => {};
		const gate = new Promise((resolve) => (release = resolve));
		vi.stubGlobal(
			"fetch",
			vi.fn(async () => {
				await gate;
				return { ok: true, headers: { get: () => "1" }, text: async () => "<h1>docs</h1>" };
			}),
		);

		navigation();
		click(link);
		await vi.waitFor(() => expect(document.documentElement.getAttribute("aria-busy")).toBe("true"));
		release(null);
		await vi.waitFor(() => expect(document.documentElement.hasAttribute("aria-busy")).toBe(false));
	});
});

describe("deferred slots", () => {
	function slot(name: string): HTMLElement {
		const element = document.createElement("gopage-slot");
		element.setAttribute("name", name);
		element.textContent = "waiting";
		document.body.append(element);
		return element;
	}

	function template(name: string, html: string): HTMLTemplateElement {
		const element = document.createElement("template");
		element.setAttribute("data-gopage-slot", name);
		element.innerHTML = html;
		document.body.append(element);
		return element;
	}

	it("moves a template into its slot", () => {
		const target = slot("Reviews");
		template("Reviews", "<b>late</b>");

		slots();

		expect(target.innerHTML).toBe("<b>late</b>");
		expect(document.querySelectorAll("template[data-gopage-slot]").length).toBe(0);
	});

	it("hydrates an island that arrives inside a slot", async () => {
		const mount = vi.fn(() => () => {});
		register("Late", async () => ({ mount }));
		const target = slot("Reviews");
		const element = document.createElement("gopage-island");
		element.setAttribute("name", "Late");
		element.setAttribute("strategy", "load");
		const holder = document.createElement("template");
		holder.setAttribute("data-gopage-slot", "Reviews");
		holder.content.append(element);
		document.body.append(holder);

		slots();
		await Promise.resolve();
		await Promise.resolve();

		expect(target.querySelector("gopage-island")).not.toBeNull();
		expect(mount).toHaveBeenCalled();
	});

	it("drops a template whose slot is gone", () => {
		template("Absent", "<b>orphan</b>");

		slots();

		expect(document.querySelectorAll("template[data-gopage-slot]").length).toBe(0);
		expect(document.body.innerHTML).not.toContain("orphan");
	});

	it("ignores a template without a name", () => {
		const element = document.createElement("template");
		element.setAttribute("data-gopage-slot", "");
		document.body.append(element);

		slots();

		expect(document.querySelectorAll("template").length).toBe(1);
	});
})

describe("hydration while the document streams", () => {
	it("mounts an island the parser has already closed", async () => {
		const mount = vi.fn();
		register("Closed", async () => ({ mount }));
		island("Closed", "load", {});
		document.body.append(document.createElement("p"));
		vi.spyOn(document, "readyState", "get").mockReturnValue("loading");

		start();
		await vi.waitFor(() => expect(mount).toHaveBeenCalledTimes(1));
	});

	it("leaves an island the parser is still inside alone", async () => {
		const mount = vi.fn();
		register("Open", async () => ({ mount }));
		island("Open", "load", {});
		vi.spyOn(document, "readyState", "get").mockReturnValue("loading");

		start();
		expect(mount).not.toHaveBeenCalled();

		document.dispatchEvent(new Event("DOMContentLoaded"));
		await vi.waitFor(() => expect(mount).toHaveBeenCalledTimes(1));
	});

	it("mounts an island that arrives in a slot while the document streams", async () => {
		const mount = vi.fn();
		register("Slotted", async () => ({ mount }));
		vi.spyOn(document, "readyState", "get").mockReturnValue("loading");
		start();

		const slot = document.createElement("gopage-slot");
		slot.setAttribute("name", "Latest");
		document.body.append(slot);
		const template = document.createElement("template");
		template.setAttribute("data-gopage-slot", "Latest");
		template.innerHTML = `<gopage-island name="Slotted" strategy="load"></gopage-island>`;
		document.body.append(template);

		await vi.waitFor(() => expect(mount).toHaveBeenCalledTimes(1));
	});
});

describe("hydration without DOMContentLoaded", () => {
	it("mounts an island once the parser moves past it", async () => {
		const mount = vi.fn();
		register("Late", async () => ({ mount }));
		vi.spyOn(document, "readyState", "get").mockReturnValue("loading");
		start();

		island("Late", "load", {});
		expect(mount).not.toHaveBeenCalled();

		document.body.append(document.createElement("p"));
		await vi.waitFor(() => expect(mount).toHaveBeenCalledTimes(1));
	});
})

describe("fetched fragments", () => {
	function slot(name: string, placeholder = "<i>waiting</i>"): HTMLElement {
		const element = document.createElement("gopage-slot");
		element.setAttribute("name", name);
		element.setAttribute("fetch", "");
		element.innerHTML = placeholder;
		document.body.append(element);
		return element;
	}

	function answer(body: string, init: Record<string, unknown> = {}): void {
		vi.stubGlobal(
			"fetch",
			vi.fn(async () => ({
				ok: true,
				headers: { get: () => "text/vnd.gopage-fragment" },
				text: async () => body,
				...init,
			})),
		);
	}

	it("replaces the placeholder with the fragment body", async () => {
		const element = slot("Latest");
		answer("<p>late</p>");

		pull();
		await vi.waitFor(() => expect(element.innerHTML).toBe("<p>late</p>"));
	});

	it("asks for the fragment by name on the current path", async () => {
		history.replaceState(null, "", "/listings/7");
		slot("Reviews");
		answer("<p>ok</p>");

		pull();
		await vi.waitFor(() => expect(fetch).toHaveBeenCalled());
		const [href, init] = (fetch as unknown as { mock: { calls: [string, RequestInit][] } }).mock.calls[0];
		expect(href).toBe("/listings/7");
		expect((init.headers as Record<string, string>)["GOPAGE-Fragment"]).toBe("Reviews");
	});

	it("hydrates an island that arrives inside a fragment", async () => {
		const mount = vi.fn();
		register("Inside", async () => ({ mount }));
		slot("Latest");
		answer(`<gopage-island name="Inside" strategy="load"></gopage-island>`);

		pull();
		await vi.waitFor(() => expect(mount).toHaveBeenCalledTimes(1));
	});

	it("waits for the slot to come into view before asking for it", async () => {
		const seen = observers();
		const element = slot("Latest");
		element.setAttribute("fetch", "visible");
		answer("<p>late</p>");

		pull();
		expect(fetch).not.toHaveBeenCalled();
		expect(seen.observed).toContain(element);
		expect(element.innerHTML).toBe("<i>waiting</i>");

		seen.fire();
		await vi.waitFor(() => expect(element.innerHTML).toBe("<p>late</p>"));
		expect(fetch).toHaveBeenCalledTimes(1);
	});

	it("observes a lazy slot once, however often pull runs", async () => {
		const seen = observers();
		const element = slot("Latest");
		element.setAttribute("fetch", "visible");
		answer("<p>late</p>");

		pull();
		pull();
		expect(seen.observed.filter((node) => node === element)).toHaveLength(1);

		seen.fire();
		await vi.waitFor(() => expect(fetch).toHaveBeenCalledTimes(1));
	});

	it("asks straight away when the environment cannot observe", async () => {
		vi.stubGlobal("IntersectionObserver", undefined);
		const element = slot("Latest");
		element.setAttribute("fetch", "visible");
		answer("<p>late</p>");

		pull();
		await vi.waitFor(() => expect(element.innerHTML).toBe("<p>late</p>"));
	});

	it("keeps the placeholder when the answer is not a fragment", async () => {
		const element = slot("Latest");
		vi.stubGlobal("fetch", vi.fn(async () => ({ ok: false, headers: { get: () => "text/html" }, text: async () => "" })));

		pull();
		await vi.waitFor(() => expect(element.hasAttribute("data-failed")).toBe(true));
		expect(element.innerHTML).toBe("<i>waiting</i>");
	});

	it("asks for each slot once", async () => {
		slot("Latest");
		answer("<p>late</p>");

		pull();
		pull();
		await vi.waitFor(() => expect(fetch).toHaveBeenCalledTimes(1));
	});

	it("leaves a slot without the attribute alone", async () => {
		const element = document.createElement("gopage-slot");
		element.setAttribute("name", "Tail");
		document.body.append(element);
		answer("<p>late</p>");

		pull();
		await new Promise((resolve) => setTimeout(resolve, 20));
		expect(fetch).not.toHaveBeenCalled();
	});
});
