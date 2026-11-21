import type { Database } from "bun:sqlite"
import { createHash, randomBytes } from "node:crypto"
import { readFile, rename, rm, writeFile } from "node:fs/promises"
import { join } from "node:path"

// The tokens devices connect to the server with: one long random secret per
// device, each with a name such as "laptop" or "phone", so that one can be
// taken back without the others. The secret is shown once when it is made;
// the database keeps only its name, when it was made, and a hash of it.
export type Tokens = {
  // Makes a token for a device and returns its secret. A name that is
  // already in use is refused, with what to do instead.
  add(name: string): { secret: string } | { refused: string }
  // The tokens there are, oldest first.
  list(): { name: string; created: number }[]
  // Takes a token back. Says whether there was one with that name.
  remove(name: string): boolean
  // The name of the device a secret belongs to, or undefined when it is not
  // a token there is.
  check(secret: string): string | undefined
}

const hash = (secret: string) => createHash("sha256").update(secret).digest("hex")

// Works on the `token` table of the database it is given. The server and the
// token commands each open the database, so a token taken back by the
// command is refused by a server that is already running.
export function createTokens(db: Database): Tokens {
  return {
    add(name) {
      const secret = randomBytes(32).toString("base64url")
      const { changes } = db.run("INSERT INTO token (name, hash, created) VALUES (?, ?, ?) ON CONFLICT (name) DO NOTHING", [name, hash(secret), Date.now()])
      if (changes === 0) return { refused: `There is already a token named "${name}". Remove it first with \`evie token remove ${name}\`.` }
      return { secret }
    },
    list: () => db.query<{ name: string; created: number }, []>("SELECT name, created FROM token ORDER BY created, name").all(),
    remove: (name) => db.run("DELETE FROM token WHERE name = ?", [name]).changes > 0,
    check: (secret) => db.query<{ name: string }, [string]>("SELECT name FROM token WHERE hash = ?").get(hash(secret))?.name,
  }
}

// This computer's own token, kept in a file in the data folder that only the
// owner's user can read, so that `evie` here connects with nothing to set
// up. It is no weaker than the keys in `.env` beside it: whatever can read
// one can read the other.
const OWN_NAME = "this computer"
const OWN_FILE = "server-token"

// Makes this computer's token when it has none that works: the file is
// missing, or holds a token that was taken back. Called when the server
// starts.
export async function ensureOwnToken(tokens: Tokens, home: string): Promise<void> {
  const saved = await readOwnToken(home)
  if (saved !== undefined && tokens.check(saved) !== undefined) return
  tokens.remove(OWN_NAME)
  const made = tokens.add(OWN_NAME)
  if ("refused" in made) throw new Error(made.refused)
  // Written beside the file, readable by the owner only, and then moved into
  // place, so the secret is never in a file others can read.
  const file = join(home, OWN_FILE)
  await rm(`${file}.new`, { force: true })
  await writeFile(`${file}.new`, `${made.secret}\n`, { mode: 0o600 })
  await rename(`${file}.new`, file)
}

// This computer's token, or undefined when the server has not made one yet.
export async function readOwnToken(home: string): Promise<string | undefined> {
  const text = await readFile(join(home, OWN_FILE), "utf8").catch(() => "")
  return text.trim() || undefined
}
