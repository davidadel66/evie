import { compare } from "./diff.ts"

// The thread lines are compared on, apart from the one that serves every
// session, so that a long comparison holds nothing else up. It is given
// one file at a time, and answers each by its number.
declare const self: Worker
self.onmessage = (event: MessageEvent<{ id: number; path: string; before: Uint8Array; after: Uint8Array; patch: boolean }>) => {
  const { id, path, before, after, patch } = event.data
  self.postMessage({ id, file: compare(path, before, after, { patch }) })
}
