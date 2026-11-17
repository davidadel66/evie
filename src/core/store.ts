import { Database } from "bun:sqlite"

// Opens Evie's one database file and creates its tables when they are missing.
export function openStore(path: string): Database {
  const db = new Database(path, { create: true, strict: true })
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
  return db
}
