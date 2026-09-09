import { delimiter, join } from "node:path";
import { describe, expect, it } from "vitest";
import { type Lookup, resolveBinary } from "./binary";

function lookup(present: string[], overrides: Partial<Lookup> = {}): Lookup {
	return {
		configured: "",
		folders: [],
		searchPath: "",
		platform: "linux",
		exists: candidate => present.includes(candidate),
		...overrides,
	};
}

describe("resolveBinary", () => {
	it("trusts a configured path without touching the disk", () => {
		expect(resolveBinary(lookup([], { configured: "/opt/gopage" }))).toBe("/opt/gopage");
	});

	it("treats a blank setting as unset", () => {
		const path = join("/usr/bin", "gopage");
		expect(resolveBinary(lookup([path], { configured: "   ", searchPath: "/usr/bin" }))).toBe(path);
	});

	it("prefers a workspace binary over one on the search path", () => {
		const local = join("/work", "node_modules", ".bin", "gopage");
		const global = join("/usr/bin", "gopage");
		const found = resolveBinary(
			lookup([local, global], { folders: ["/work"], searchPath: "/usr/bin" }),
		);
		expect(found).toBe(local);
	});

	it("walks every workspace folder", () => {
		const second = join("/b", "node_modules", ".bin", "gopage");
		expect(resolveBinary(lookup([second], { folders: ["/a", "/b"] }))).toBe(second);
	});

	it("walks the search path in order and skips empty entries", () => {
		const second = join("/second", "gopage");
		const searchPath = ["/first", "", "/second"].join(delimiter);
		expect(resolveBinary(lookup([second], { searchPath }))).toBe(second);
	});

	it("finds nothing when nothing is there", () => {
		expect(resolveBinary(lookup([], { folders: ["/work"], searchPath: "/usr/bin" }))).toBeUndefined();
	});

	it("looks for the windows executable first", () => {
		const exe = join("/tools", "gopage.exe");
		const bare = join("/tools", "gopage");
		const found = resolveBinary(
			lookup([exe, bare], { searchPath: "/tools", platform: "win32" }),
		);
		expect(found).toBe(exe);
	});

	it("falls back to the windows shim a package manager writes", () => {
		const shim = join("/work", "node_modules", ".bin", "gopage.cmd");
		const found = resolveBinary(lookup([shim], { folders: ["/work"], platform: "win32" }));
		expect(found).toBe(shim);
	});
});
