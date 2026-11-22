import { noAnswer, noServer, refusal } from "../server/connect.ts"
import { PATHS } from "../server/wire.ts"
import type { Settings } from "../index.ts"
import { fail, print, reachServer } from "./common.ts"

// `evie web`: opens the web app in the browser, already signed in. It asks
// the server for a sign-in link, which works once, within five minutes, and
// gives the browser that opens it a token of its own. `--print` prints the
// link instead, to open by hand.
export async function web(env: Settings, args: string[]) {
  if (args.some((arg) => arg !== "--print")) {
    fail(
      [
        "Usage:",
        "  evie web           open the web app in your browser, signed in",
        "  evie web --print   print the sign-in link instead, to open by hand",
      ].join("\n"),
    )
  }
  const { address, token, own, why } = await reachServer(env)
  // The server gives sign-in links to this computer only: a browser
  // elsewhere signs in with a token of its own.
  if (!own) fail(`\`evie web\` works on the computer evie's server runs on. Here, open ${address} in a browser and paste this device's token.`)
  const link = await signInLink(new URL(address), token).catch((cause) => fail(why(cause)))
  if (args.includes("--print")) return print(`${link}\n`)

  const opener = process.platform === "darwin" ? "open" : "xdg-open"
  try {
    const opened = Bun.spawn([opener, link], { stdin: "ignore", stdout: "ignore", stderr: "ignore" })
    if ((await opened.exited) === 0) return print("Opened the web app in your browser.\n")
  } catch {
    // There is no such program here, as on a server with no desktop.
  }
  print(`No browser could be opened with \`${opener}\`. Open this link in one within five minutes; it works once:\n\n  ${link}\n`)
}

// Asks the server for a link that signs a browser in, with this device's token.
async function signInLink(address: URL, token: string): Promise<string> {
  const signal = AbortSignal.timeout(10_000)
  let answer: Response
  try {
    answer = await fetch(new URL(PATHS.signInLink, address), { method: "POST", headers: { Authorization: `Bearer ${token}` }, signal })
  } catch {
    throw signal.aborted ? noAnswer(address) : noServer(address)
  }
  if (!answer.ok) throw refusal(address, answer.status, true)
  return ((await answer.json()) as { link: string }).link
}
