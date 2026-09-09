import * as assert from "node:assert";
import * as path from "node:path";
import * as vscode from "vscode";

const identifier = "apptivitypl.gopage";

async function wakes(extension: vscode.Extension<unknown>): Promise<boolean> {
	for (let attempt = 0; attempt < 100; attempt++) {
		if (extension.isActive) {
			return true;
		}
		await new Promise(resolve => setTimeout(resolve, 100));
	}
	return false;
}

suite("the extension", () => {
	test("is installed and starts out asleep", () => {
		const extension = vscode.extensions.getExtension(identifier);
		assert.ok(extension, "the extension is not installed");
		assert.strictEqual(extension.isActive, false, "something activated it before the test did");
	});

	test("claims .gopage files and wakes up for them", async () => {
		const file = path.join(
			vscode.workspace.workspaceFolders![0].uri.fsPath,
			"app",
			"page.gopage",
		);
		const document = await vscode.workspace.openTextDocument(file);
		assert.strictEqual(document.languageId, "gopage");
		await vscode.window.showTextDocument(document);
		const extension = vscode.extensions.getExtension(identifier)!;
		assert.ok(await wakes(extension), "opening a .gopage file did not wake the extension");
	});

	test("registers every command it contributes", async () => {
		const registered = await vscode.commands.getCommands(true);
		for (const command of ["gopage.build", "gopage.dev", "gopage.restartServer"]) {
			assert.ok(registered.includes(command), `${command} is not registered`);
		}
	});

	test("ships the settings the readme documents", () => {
		const settings = vscode.workspace.getConfiguration("gopage");
		assert.strictEqual(settings.get("path"), "");
		assert.strictEqual(settings.get("languageServer.enabled"), true);
		assert.strictEqual(settings.get("trace.server"), "off");
	});

	test("survives a missing binary", async () => {
		await vscode.commands.executeCommand("gopage.restartServer");
	});
});
