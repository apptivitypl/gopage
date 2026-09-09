import { type ConfigurationChangeEvent, type ExtensionContext, commands, window, workspace } from "vscode";
import type { LanguageClient } from "vscode-languageclient/node";
import { createClient, enabled, locate, section } from "./client";
import { run } from "./tasks";

let client: LanguageClient | undefined;

const missing = "gopage is not on PATH and no gopage.path is set, so highlighting works but diagnostics do not.";

async function withBinary(action: (binary: string) => Promise<void>): Promise<void> {
	const binary = locate();
	if (binary === undefined) {
		await report();
		return;
	}
	await action(binary);
}

async function report(): Promise<void> {
	const settings = "Open settings";
	const chosen = await window.showWarningMessage(missing, settings);
	if (chosen === settings) {
		await commands.executeCommand("workbench.action.openSettings", `${section}.path`);
	}
}

function reason(failure: unknown): string {
	return failure instanceof Error ? failure.message : String(failure);
}

async function start(): Promise<void> {
	if (client !== undefined || !enabled()) {
		return;
	}
	const binary = locate();
	if (binary === undefined) {
		return;
	}
	const starting = createClient(binary);
	try {
		await starting.start();
		client = starting;
	} catch (failure) {
		await window.showWarningMessage(`${binary} lsp did not start: ${reason(failure)}`);
	}
}

async function stop(): Promise<void> {
	const running = client;
	client = undefined;
	await running?.stop();
}

async function restart(): Promise<void> {
	await stop();
	await start();
}

function reconfigured(change: ConfigurationChangeEvent): boolean {
	return [`${section}.path`, `${section}.languageServer.enabled`].some(key =>
		change.affectsConfiguration(key),
	);
}

export async function activate(context: ExtensionContext): Promise<void> {
	context.subscriptions.push(
		commands.registerCommand("gopage.build", () => withBinary(binary => run("build", binary))),
		commands.registerCommand("gopage.dev", () => withBinary(binary => run("dev", binary))),
		commands.registerCommand("gopage.restartServer", restart),
		workspace.onDidChangeConfiguration(async change => {
			if (reconfigured(change)) {
				await restart();
			}
		}),
	);
	await start();
}

export async function deactivate(): Promise<void> {
	await stop();
}
