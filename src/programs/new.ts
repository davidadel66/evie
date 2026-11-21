import { join, resolve } from "node:path"
import { connect, type Settings } from "../index.ts"
import { fail, print, reachServer } from "./common.ts"

// `evie new <folder> [name]`: makes a project through the server, as the
// web UI will. The folder is taken from where the command is run, and is
// made when it is missing; the project is named after it unless a name is
// given. The server makes the project's `.evie/evie.json`, which backs up
// its `.evie/` folder to a private GitHub repository, and trusts it.
export async function newProject(env: Settings, [folder, ...words]: string[]) {
  if (folder === undefined || folder.trim() === "") {
    fail(["Usage:", "  evie new <folder> [name]   make a project in a folder, made when it is missing, named after it unless a name is given"].join("\n"))
  }
  const name = words.join(" ").trim()
  const { address, token, why } = await reachServer(env)
  const evie = await connect({ url: address, token, signal: AbortSignal.timeout(10_000) }).catch((cause) => fail(why(cause)))
  let problem: unknown
  try {
    const { project, madeFolder } = await evie.send({ type: "new_project", folder: resolve(folder!), ...(name !== "" && { name }) })
    print(
      `${madeFolder ? `Made ${project.folder}, and the project "${project.name}" in it` : `Made the project "${project.name}" in ${project.folder}`}, which is trusted.\n` +
        `Its settings are in ${join(project.folder, ".evie", "evie.json")}, which holds "sync": "private": its .evie folder is backed ` +
        "up to a private GitHub repository, made the first time it syncs.\n" +
        "Run `evie` in that folder to start working there.\n",
    )
  } catch (cause) {
    problem = cause
  } finally {
    await evie.close()
  }
  if (problem !== undefined) fail(problem)
}
