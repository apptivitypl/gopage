import { ShellExecution, Task, TaskScope, tasks, workspace } from "vscode";

export async function run(command: string, binary: string): Promise<void> {
	const folder = workspace.workspaceFolders?.[0];
	const task = new Task(
		{ type: "gopage", command },
		folder ?? TaskScope.Workspace,
		command,
		"gopage",
		new ShellExecution(binary, [command]),
	);
	await tasks.executeTask(task);
}
