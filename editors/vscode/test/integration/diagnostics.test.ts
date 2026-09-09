import * as assert from "node:assert";
import { rm } from "node:fs/promises";
import * as path from "node:path";
import * as vscode from "vscode";

const binary = process.env.GOPAGE_BINARY ?? "";

function fixture(name: string): vscode.Uri {
	return vscode.Uri.file(
		path.join(vscode.workspace.workspaceFolders![0].uri.fsPath, "app", name),
	);
}

async function settles(uri: vscode.Uri): Promise<vscode.Diagnostic[]> {
	for (let attempt = 0; attempt < 150; attempt++) {
		const reported = vscode.languages.getDiagnostics(uri);
		if (reported.length > 0) {
			return reported;
		}
		await new Promise(resolve => setTimeout(resolve, 100));
	}
	return [];
}

suite("the language server", function () {
	suiteSetup(async function () {
		if (binary === "") {
			this.skip();
		}
		await vscode.workspace
			.getConfiguration("gopage")
			.update("path", binary, vscode.ConfigurationTarget.Workspace);
	});

	suiteTeardown(async () => {
		if (binary === "") {
			return;
		}
		await vscode.workspace
			.getConfiguration("gopage")
			.update("path", undefined, vscode.ConfigurationTarget.Workspace);
		await rm(
			path.join(vscode.workspace.workspaceFolders![0].uri.fsPath, ".vscode"),
			{ recursive: true, force: true },
		);
	});

	test("reports what the compiler would refuse", async () => {
		const uri = fixture("broken.gopage");
		await vscode.window.showTextDocument(await vscode.workspace.openTextDocument(uri));
		const reported = await settles(uri);
		assert.ok(reported.length > 0, "the server reported nothing");
		const codes = reported.map(one => String(one.code));
		assert.ok(
			codes.some(code => code.startsWith("C")),
			`no gopage diagnostic code among ${codes.join(", ")}`,
		);
		assert.ok(reported.every(one => one.source === "gopage"));
	});

	test("says nothing about a page that compiles", async () => {
		const uri = fixture("page.gopage");
		await vscode.window.showTextDocument(await vscode.workspace.openTextDocument(uri));
		await new Promise(resolve => setTimeout(resolve, 1500));
		assert.deepStrictEqual(vscode.languages.getDiagnostics(uri), []);
	});
});
