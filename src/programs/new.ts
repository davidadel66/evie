import { join, resolve } from "node:path"
import { connect, type Settings } from "../index.ts"
import { fail, print, reachServer } from "./common.ts"

// `evie new <folder> [name]`: makes a project through the server, as the
// web UI will. The folder is taken from where the command is run, and is
// made when it is missing; the project is named after it unless a name is
// given. The server makes the project's `.evie/evie.json`, which backs up
// its `.evie/` folder to a private GitHub repository, and trusts it unless
// that folder already held files. What it prints says which of these hold.
export async function newProject(env: Settings, [folder, ...words]: string[]) {
  if (folder === undefined || folder.trim() === "") {
    fail(["Usage:", "  evie new <folder> [name]   make a project in a folder, made when it is missing, named after it unless a name is given"].join("\n"))
  }
  const name = words.join(" ").trim()
  const { address, token, why } = await reachServer(env)
  const evie = await connect({ url: address, token, signal: AbortSignal.timeout(10_000) }).catch((cause) => fail(why(cause)))
  let problem: unknown
  try {
    const { project, madeFolder, trusted, syncs } = await evie.send({ type: "new_project", folder: resolve(folder!), ...(name !== "" && { name }) })
    const made = madeFolder ? `Made ${project.folder}, and the project "${project.name}" in it` : `Made the project "${project.name}" in ${project.folder}`
    const settings = `Its settings are in ${join(project.folder, ".evie", "evie.json")}, which holds "sync": "private"`
    print(
      (trusted ? `${made}, which is trusted.\n` : `${made}. It is not trusted yet, because its .evie folder already held files, which came with it.\n`) +
        (!trusted
          ? `${settings}. Like the rest of its own settings, that is used only once you trust the project, which its first session asks.\n`
          : syncs
            ? `${settings}: its .evie folder is backed up to a private GitHub repository, made the first time it syncs.\n`
            : `${settings}, but the server does not load the sync plugin, so its .evie folder is not backed up. To back it up, ` +
              `add "sync" to "plugins" in the server's evie.json and start the server again.\n`) +
        "Run `evie` in that folder to start working there.\n",
    )
  } catch (cause) {
    problem = cause
  } finally {
    await evie.close()
  }
  if (problem !== undefined) fail(problem)
}
