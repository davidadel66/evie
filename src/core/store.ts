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
    const applied = db.query<{ user_version: number }, []>("PRAGMA user_version").get()?.user_version ?? 0
    if (applied > migrations.length) {
      throw new Error(
        `The database at ${path} was written by a newer version of Evie ` +
          `(its tables are at version ${applied}; this Evie understands up to ${migrations.length}). ` +
          `Update Evie to open it.`,
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
