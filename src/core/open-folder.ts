// Opens a folder on the computer evie runs on, in its file manager, as T3
// Code's "Open in" opens a thread's folder: with `open` on macOS and
// `xdg-open` on Linux, given the folder as an argument, with no shell.
// Gives back once the program has taken it, or fails, saying why.
export async function openOnThisComputer(folder: string, platform: NodeJS.Platform = process.platform): Promise<void> {
  const program = platform === "darwin" ? "open" : platform === "linux" ? "xdg-open" : undefined
  if (program === undefined) throw new Error(`evie does not know how to open a folder on ${platform}.`)
  let run
  try {
    run = Bun.spawn([program, folder], { stdin: "ignore", stdout: "ignore", stderr: "pipe", timeout: 10_000 })
  } catch {
    throw new Error(`The folder could not be opened: ${program} is not on the computer evie runs on.`)
  }
  const [problem, code] = await Promise.all([new Response(run.stderr).text(), run.exited])
  if (code !== 0) throw new Error(`The folder could not be opened: ${problem.trim() || `${program} stopped with code ${code}.`}`)
}
