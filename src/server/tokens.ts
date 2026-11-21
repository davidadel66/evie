import type { Database } from "bun:sqlite"
import { createHash, randomBytes } from "node:crypto"

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
