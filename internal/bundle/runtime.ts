type Mount = (element: HTMLElement, props: Record<string, unknown>) => (() => void) | void;

type Island = {
	element: HTMLElement;
	name: string;
	strategy: string;
	media: string | null;
};

const modules: Record<string, () => Promise<{ mount: Mount }>> = {};
const mounted = new WeakMap<HTMLElement, () => void>();
let parsing = false;

export function register(name: string, load: () => Promise<{ mount: Mount }>): void {
	modules[name] = load;
}

export function hydrate(root: ParentNode = document, complete = false): void {
	for (const element of root.querySelectorAll<HTMLElement>("rill-island")) {
		const island = read(element, complete);
		if (island) {
			activate(island);
		}
	}
}

export function release(root: ParentNode = document): void {
	for (const element of root.querySelectorAll<HTMLElement>("rill-island")) {
		const stop = mounted.get(element);
		if (stop) {
			stop();
			mounted.delete(element);
		}
	}
}

function read(element: HTMLElement, complete: boolean): Island | null {
	const name = element.getAttribute("name");
	if (!name || mounted.has(element) || !(complete || parsed(element))) {
		return null;
	}
	return {
		element,
		name,
		strategy: element.getAttribute("strategy") ?? "load",
		media: element.getAttribute("media"),
	};
}

function parsed(element: HTMLElement): boolean {
	if (!parsing) {
		return true;
	}
	for (let node: Node | null = element; node; node = node.parentNode) {
		if (node.nextSibling) {
			return true;
		}
	}
	return false;
}

function activate(island: Island): void {
	switch (island.strategy) {
		case "idle":
			whenIdle(() => mount(island));
			return;
		case "visible":
			whenVisible(box(island.element), () => mount(island));
			return;
		case "media":
			whenMedia(island.media, () => mount(island));
			return;
		default:
			mount(island);
	}
}

function box(element: HTMLElement): HTMLElement {
	for (const child of element.children) {
		if (child instanceof HTMLElement && child.tagName !== "SCRIPT") {
			return child;
		}
	}
	return element;
}

function whenIdle(run: () => void): void {
	const idle = (window as unknown as { requestIdleCallback?: (cb: () => void) => void }).requestIdleCallback;
	if (idle) {
		idle(run);
		return;
	}
	setTimeout(run, 1);
}

function whenVisible(element: HTMLElement, run: () => void): void {
	if (typeof IntersectionObserver === "undefined") {
		run();
		return;
	}
	const observer = new IntersectionObserver((entries) => {
		for (const entry of entries) {
			if (entry.isIntersecting) {
				observer.disconnect();
				run();
				return;
			}
		}
	});
	observer.observe(element);
}

function whenMedia(query: string | null, run: () => void): void {
	if (!query || typeof matchMedia === "undefined") {
		run();
		return;
	}
	const list = matchMedia(query);
	if (list.matches) {
		run();
		return;
	}
	const onChange = () => {
		if (list.matches) {
			list.removeEventListener("change", onChange);
			run();
		}
	};
	list.addEventListener("change", onChange);
}

async function mount(island: Island): Promise<void> {
	const load = modules[island.name];
	if (!load || mounted.has(island.element)) {
		return;
	}
	mounted.set(island.element, () => {});
	const module = await load();
	const stop = module.mount(island.element, props(island.element));
	mounted.set(island.element, typeof stop === "function" ? stop : () => {});
}

function props(element: HTMLElement): Record<string, unknown> {
	const script = element.querySelector<HTMLScriptElement>('script[type="application/json"]');
	if (!script?.textContent) {
		return {};
	}
	try {
		return JSON.parse(script.textContent) as Record<string, unknown>;
	} catch {
		return {};
	}
}

export function start(): void {
	if (typeof document === "undefined") {
		return;
	}
	parsing = document.readyState === "loading";
	slots();
	hydrate();
	pull();
	if (parsing) {
		document.addEventListener("DOMContentLoaded", () => {
			parsing = false;
			hydrate();
		}, { once: true });
	}
}

const SLOT_ATTRIBUTE = "data-rill-slot";

export function slots(root: ParentNode = document): void {
	for (const template of root.querySelectorAll<HTMLTemplateElement>(`template[${SLOT_ATTRIBUTE}]`)) {
		fill(template);
	}
	if (typeof MutationObserver === "undefined" || document.readyState === "complete") {
		return;
	}
	const observer = new MutationObserver((records) => {
		for (const record of records) {
			for (const node of record.addedNodes) {
				if (node instanceof HTMLTemplateElement && node.hasAttribute(SLOT_ATTRIBUTE)) {
					fill(node);
				}
			}
		}
		sweep();
	});
	observer.observe(document.documentElement, { childList: true, subtree: true });
	window.addEventListener("load", () => observer.disconnect(), { once: true });
}

let scheduled = false;

function sweep(): void {
	if (scheduled || !parsing) {
		return;
	}
	scheduled = true;
	queueMicrotask(() => {
		scheduled = false;
		hydrate();
		pull();
	});
}

const FRAGMENT_HEADER = "RILL-Fragment";
const FRAGMENT_TYPE = "text/vnd.rill-fragment";
const PULLED_ATTRIBUTE = "data-rill-pulled";

let fragments: AbortController | null = null;
const awaited = new WeakSet<HTMLElement>();

export function pull(root: ParentNode = document): void {
	if (typeof fetch === "undefined") {
		return;
	}
	for (const slot of root.querySelectorAll<HTMLElement>("rill-slot[fetch]")) {
		if (slot.hasAttribute(PULLED_ATTRIBUTE) || awaited.has(slot)) {
			continue;
		}
		switch (slot.getAttribute("fetch")) {
			case "visible":
				awaited.add(slot);
				whenVisible(slot, () => void draw(slot));
				break;
			case "idle":
				awaited.add(slot);
				whenIdle(() => void draw(slot));
				break;
			default:
				void draw(slot);
		}
	}
}

function abandon(): void {
	fragments?.abort();
	fragments = null;
}

async function draw(slot: HTMLElement): Promise<void> {
	const name = slot.getAttribute("name");
	if (!name) {
		return;
	}
	slot.setAttribute(PULLED_ATTRIBUTE, "");
	fragments ??= new AbortController();
	const controller = fragments;
	try {
		const response = await fetch(location.pathname + location.search, {
			headers: { [FRAGMENT_HEADER]: name },
			signal: controller.signal,
		});
		const type = response.headers.get("content-type") ?? "";
		if (!response.ok || !type.startsWith(FRAGMENT_TYPE)) {
			throw new Error("not a fragment");
		}
		const html = await response.text();
		if (controller.signal.aborted || !slot.isConnected) {
			return;
		}
		const holder = document.createElement("template");
		holder.innerHTML = html;
		slot.replaceChildren(holder.content);
		hydrate(slot as ParentNode, true);
	} catch {
		if (!controller.signal.aborted) {
			slot.setAttribute("data-failed", "");
		}
	}
}

function fill(template: HTMLTemplateElement): void {
	const name = template.getAttribute(SLOT_ATTRIBUTE);
	if (!name) {
		return;
	}
	const slot = document.querySelector(`rill-slot[name="${CSS.escape(name)}"]`);
	template.remove();
	if (!slot) {
		return;
	}
	const content = template.content;
	slot.replaceChildren(content);
	hydrate(slot as ParentNode, true);
}

const PARTIAL_HEADER = "RILL-Partial";
const PARTIAL_TYPE = "text/vnd.rill-partial";
const LEVEL_HEADER = "RILL-Level";
const TITLE_HEADER = "RILL-Title";

let generation = 0;
let pending: AbortController | null = null;
let wired = false;

export function navigation(): void {
	if (typeof document === "undefined" || wired) {
		return;
	}
	wired = true;
	document.addEventListener("click", onClick);
	window.addEventListener("popstate", () => void go(location.href, false));
}

function onClick(event: MouseEvent): void {
	if (event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) {
		return;
	}
	const link = (event.target as Element | null)?.closest?.("a");
	if (!link || !internal(link)) {
		return;
	}
	event.preventDefault();
	void go(link.href, true);
}

function internal(link: HTMLAnchorElement): boolean {
	if (link.target && link.target !== "_self") {
		return false;
	}
	if (link.hasAttribute("download") || link.dataset.rillNav === "off") {
		return false;
	}
	const url = new URL(link.href, location.href);
	return url.origin === location.origin && url.pathname !== location.pathname;
}

async function go(href: string, push: boolean): Promise<void> {
	const mine = ++generation;
	pending?.abort();
	abandon();
	const controller = new AbortController();
	pending = controller;
	document.documentElement.setAttribute("aria-busy", "true");
	try {
		const from = location.pathname;
		const response = await fetch(href, {
			headers: { [PARTIAL_HEADER]: from },
			signal: controller.signal,
		});
		const type = response.headers.get("content-type") ?? "";
		if (!response.ok || !type.startsWith(PARTIAL_TYPE)) {
			throw new Error("not a partial");
		}
		if (mine !== generation) {
			return;
		}
		const level = Number(response.headers.get(LEVEL_HEADER) ?? "0");
		const html = await response.text();
		if (mine !== generation) {
			return;
		}
		if (!swap(level, html)) {
			throw new Error("no outlet");
		}
		const title = response.headers.get(TITLE_HEADER);
		if (title !== null) {
			document.title = decodeURIComponent(title.replace(/\+/g, " "));
		}
		if (push) {
			history.pushState(null, "", response.url || href);
		}
		scrollTo({ top: 0, behavior: reduced() ? "auto" : "smooth" });
	} catch {
		if (mine === generation) {
			location.href = href;
		}
	} finally {
		if (mine === generation) {
			document.documentElement.removeAttribute("aria-busy");
			pending = null;
		}
	}
}

function reduced(): boolean {
	return typeof matchMedia !== "undefined" && matchMedia("(prefers-reduced-motion: reduce)").matches;
}

function swap(level: number, html: string): boolean {
	if (level === 0) {
		return false;
	}
	const start = marker(`rill:o${level - 1}`);
	if (!start) {
		return false;
	}
	const end = marker(`/rill:o${level - 1}`);
	if (!end) {
		return false;
	}
	release(rangeRoot(start, end));
	const range = document.createRange();
	range.setStartAfter(start);
	range.setEndBefore(end);
	range.deleteContents();
	range.insertNode(range.createContextualFragment(html));
	hydrate(document);
	pull();
	return true;
}

function rangeRoot(start: Comment, end: Comment): ParentNode {
	return start.parentNode ?? end.parentNode ?? document;
}

function marker(text: string): Comment | null {
	const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_COMMENT);
	while (walker.nextNode()) {
		const node = walker.currentNode as Comment;
		if (node.data === text) {
			return node;
		}
	}
	return null;
}
