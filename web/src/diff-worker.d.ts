// The diff library's worker script, imported as a file: the bundler serves
// it beside the page and gives back the path it is served at.
declare module "@pierre/diffs/worker/worker-portable.js" {
  const path: string
  export default path
}
