import { Database } from "bun:sqlite"

// One step that changes the shape of the tables. Steps are only ever added
// to the end of the list: a step that has run on someone's data folder is
// never edited.
export type Migration = (db: Database) => void

export const MIGRATIONS: Migration[] = [
  // 1: the first tables. Written so that a database made before versions
  // were recorded, which already has these tables, is adopted as it is.
  (db) => {
    db.run(`CREATE TABLE IF NOT EXISTS project (
      id TEXT PRIMARY KEY,
      name TEXT NOT NULL,
      folder TEXT NOT NULL UNIQUE,
      created INTEGER NOT NULL
    )`)
    // A session with no project is the general session.
    db.run(`CREATE TABLE IF NOT EXISTS session (
      id TEXT PRIMARY KEY,
      project TEXT REFERENCES project (id),
      created INTEGER NOT NULL
    )`)
    db.run(`CREATE TABLE IF NOT EXISTS event (
      session TEXT NOT NULL,
      seq INTEGER NOT NULL,
      type TEXT NOT NULL,
      body TEXT NOT NULL,
      time INTEGER NOT NULL,
      PRIMARY KEY (session, seq)
    )`)
    db.run(`CREATE TABLE IF NOT EXISTS inbox (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      session TEXT NOT NULL,
      body TEXT NOT NULL,
      time INTEGER NOT NULL
    )`)
  },
  // 2: a session can have a parent: the session that started it as a subagent.
  (db) => {
    db.run("ALTER TABLE session ADD COLUMN parent TEXT")
  },
  // 3: a subagent can be one of the owner's named agents, and can be held to
  // reading only.
  (db) => {
    db.run("ALTER TABLE session ADD COLUMN agent TEXT")
    db.run("ALTER TABLE session ADD COLUMN read_only INTEGER")
  },
  // 4: the tokens devices connect to the server with. Only a hash of each
  // secret is kept, so a copy of the database lets no one in.
  (db) => {
    db.run(`CREATE TABLE token (
      name TEXT PRIMARY KEY,
      hash TEXT NOT NULL UNIQUE,
      created INTEGER NOT NULL
    )`)
  },
  // 5: the icon and the colour the owner picked for a project: a Phosphor
  // icon's name, such as "folder", and one of the design system's six
  // project colours, such as "moss". Empty until picked, while a client
  // draws the project as it chooses.
  (db) => {
    db.run("ALTER TABLE project ADD COLUMN icon TEXT")
    db.run("ALTER TABLE project ADD COLUMN colour TEXT")
  },
]

// Opens Evie's one database file and brings its tables up to date.
//
// The database records how many steps have been applied to it. Each missing
// step runs in its own transaction, so a step that fails leaves the database
// exactly as it was. A database with more steps than this Evie knows was
// written by a newer Evie, and is refused untouched.
export function openStore(path: string, migrations: readonly Migration[] = MIGRATIONS): Database {
  const db = new Database(path, { create: true, strict: true })
  try {
    // More than one process can have the database open, such as the server
    // and `evie token`. One that finds it being written to waits for its
    // turn, for up to five seconds, instead of failing at once.
    db.run("PRAGMA busy_timeout = 5000")
    const applied = db.query<{ user_version: number }, []>("PRAGMA user_version").get()?.user_version ?? 0
    if (applied > migrations.length) {
      throw new Error(
        `The database at ${path} was written by a newer version of evie ` +
          `(its tables are at version ${applied}; this evie understands up to ${migrations.length}). ` +
          `Update evie to open it.`,
      )
    }
    for (let step = applied; step < migrations.length; step++) {
      db.transaction(() => {
        migrations[step]!(db)
        db.run(`PRAGMA user_version = ${step + 1}`)
      })()
    }
    return db
  } catch (error) {
    db.close()
    throw error
  }
}
