// Runs the real git, with none of the settings of the user running the
// tests, such as signing commits, and an author of its own.
export const GIT_ENV = {
  PATH: process.env.PATH ?? "",
  GIT_CONFIG_GLOBAL: "/dev/null",
  GIT_CONFIG_NOSYSTEM: "1",
  GIT_AUTHOR_NAME: "Test",
  GIT_AUTHOR_EMAIL: "test@example.invalid",
  GIT_COMMITTER_NAME: "Test",
  GIT_COMMITTER_EMAIL: "test@example.invalid",
}

// Runs git in a folder and returns what it printed. A command that fails is
// an error, with what git said.
export async function git(args: string[], cwd: string): Promise<string> {
  const run = Bun.spawn(["git", ...args], { cwd, env: GIT_ENV, stdin: "ignore", stdout: "pipe", stderr: "pipe" })
  const [stdout, stderr, code] = await Promise.all([new Response(run.stdout).text(), new Response(run.stderr).text(), run.exited])
  if (code !== 0) throw new Error(`git ${args.join(" ")} failed: ${stderr}`)
  return stdout
}

// Runs a command with the same settings as `git` above, and returns how it
// went, as the sync plugin is told. `env` adds to them, as the settings
// the sync plugin gives git do.
export async function ran(command: string[], cwd: string, env: Record<string, string> = {}): Promise<{ code: number; stdout: string; stderr: string }> {
  const given = Object.fromEntries(Object.entries(env).filter(([name]) => name.startsWith("GIT_") || name === "SSH_ASKPASS"))
  const run = Bun.spawn(command, { cwd, env: { ...GIT_ENV, ...given }, stdin: "ignore", stdout: "pipe", stderr: "pipe" })
  const [stdout, stderr, code] = await Promise.all([new Response(run.stdout).text(), new Response(run.stderr).text(), run.exited])
  return { code, stdout, stderr }
}
