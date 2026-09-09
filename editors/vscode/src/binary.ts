import { delimiter, join } from "node:path";

export interface Lookup {
	readonly configured: string;
	readonly folders: readonly string[];
	readonly searchPath: string;
	readonly platform: string;
	exists(candidate: string): boolean;
}

function names(platform: string): string[] {
	return platform === "win32" ? ["gopage.exe", "gopage.cmd", "gopage"] : ["gopage"];
}

function firstPresent(lookup: Lookup, directory: string): string | undefined {
	for (const name of names(lookup.platform)) {
		const candidate = join(directory, name);
		if (lookup.exists(candidate)) {
			return candidate;
		}
	}
	return undefined;
}

export function resolveBinary(lookup: Lookup): string | undefined {
	const configured = lookup.configured.trim();
	if (configured !== "") {
		return configured;
	}
	for (const folder of lookup.folders) {
		const local = firstPresent(lookup, join(folder, "node_modules", ".bin"));
		if (local !== undefined) {
			return local;
		}
	}
	for (const entry of lookup.searchPath.split(delimiter)) {
		if (entry === "") {
			continue;
		}
		const found = firstPresent(lookup, entry);
		if (found !== undefined) {
			return found;
		}
	}
	return undefined;
}
