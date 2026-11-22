// A message in words for the owner, with the commands in it, written
// between backticks, shown as code.
export function Words({ text }: { text: string }) {
  return text.split("`").map((part, at) =>
    at % 2 === 1 ? (
      <code key={at} className="font-mono text-code">
        {part}
      </code>
    ) : (
      part
    ),
  )
}
