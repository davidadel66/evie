import { scriptedProvider, type ScriptedReply } from "./scripted-provider.ts"

// The scripted model as a plugin file, for running the real `evie serve`
// with no network. List this file in evie.json with the script as its
// options: { "use": "<this file>", "options": { "script": [...] } }.
export default (options: { script: ScriptedReply[] }) => scriptedProvider(options.script).plugin
