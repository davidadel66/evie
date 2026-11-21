import { dataFolder, type Settings } from "../index.ts"
import { openTokens } from "../server/tokens.ts"
import { fail, print } from "./common.ts"

// `evie token`: makes, lists and takes back the tokens devices connect with.
// It works on the data folder directly, so it works whether or not the
// server is running, and a token taken back is refused by a server already
// running.
export async function token(env: Settings, [action, ...words]: string[]) {
  const name = words.join(" ").trim()
  const tokens = await openTokens(dataFolder(env)).catch(fail)

  if (action === "add" && name !== "") {
    const made = tokens.add(name)
    if ("refused" in made) fail(made.refused)
    else {
      print(
        `A token for "${name}". It is shown only this once, so copy it now:\n\n  ${made.secret}\n\n` +
          `On that device, set EVIE_TOKEN to it, and EVIE_SERVER to this server's address.\n`,
      )
    }
  } else if (action === "list" && name === "") {
    const rows = tokens.list()
    if (rows.length === 0) print("There are no tokens. Make one with `evie token add <name>`.\n")
    const width = Math.max(0, ...rows.map((row) => row.name.length))
    for (const row of rows) print(`${row.name.padEnd(width)}   made ${new Date(row.created).toISOString().slice(0, 16).replace("T", " ")} UTC\n`)
  } else if (action === "remove" && name !== "") {
    if (!tokens.remove(name)) fail(`There is no token named "${name}". \`evie token list\` lists them.`)
    print(`The token for "${name}" is taken back. A device using it can no longer connect.\n`)
  } else {
    fail(
      [
        "Usage:",
        '  evie token add <name>      make a token for a device, such as "phone"',
        "  evie token list            list the tokens there are",
        "  evie token remove <name>   take a device's token back",
      ].join("\n"),
    )
  }
  tokens.close()
}
