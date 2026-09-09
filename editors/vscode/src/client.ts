import { accessSync, constants } from "node:fs";
import { workspace } from "vscode";
import { LanguageClient, type LanguageClientOptions, type ServerOptions } from "vscode-languageclient/node";
import { resolveBinary } from "./binary";

export const section = "gopage";

export function locate(): string | undefined {
	return resolveBinary({
		configured: workspace.getConfiguration(section).get<string>("path", ""),
		folders: (workspace.workspaceFolders ?? []).map(folder => folder.uri.fsPath),
		searchPath: process.env.PATH ?? "",
		platform: process.platform,
		exists: candidate => {
			try {
				accessSync(candidate, constants.X_OK);
				return true;
			} catch {
				return false;
			}
		},
	});
}

export function enabled(): boolean {
	return workspace.getConfiguration(section).get<boolean>("languageServer.enabled", true);
}

export function createClient(binary: string): LanguageClient {
	const server: ServerOptions = { command: binary, args: ["lsp"] };
	const options: LanguageClientOptions = {
		documentSelector: [{ scheme: "file", language: "gopage" }],
		outputChannelName: "GoPage",
	};
	return new LanguageClient(section, "GoPage", server, options);
}
