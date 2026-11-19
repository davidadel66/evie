// A small stand-in for a terminal window. It understands just what the live
// input line writes: text that wraps at the edge, carriage return, line feed,
// "move up n rows" and "clear from here to the end of the screen". It lets a
// test ask what a person would see.
export function fakeScreen(columns: number) {
  const rows: string[][] = [[]]
  let row = 0
  let column = 0

  const put = (character: string) => {
    // Writing past the right edge carries on at the start of the next row.
    if (column >= columns) {
      row += 1
      column = 0
    }
    while (rows.length <= row) rows.push([])
    const line = rows[row]!
    while (line.length < column) line.push(" ")
    line[column] = character
    column += 1
  }

  function write(text: string) {
    for (let at = 0; at < text.length; at++) {
      const character = text[at]!
      if (character === "\r") column = 0
      else if (character === "\n") {
        row += 1
        while (rows.length <= row) rows.push([])
      } else if (character === "\x1b") {
        const sequence = /^\x1b\[(\d*)([A-Za-z])/.exec(text.slice(at))
        if (!sequence) throw new Error(`the fake screen does not understand ${JSON.stringify(text.slice(at, at + 6))}`)
        at += sequence[0].length - 1
        if (sequence[2] === "A") row = Math.max(0, row - Number(sequence[1] || 1))
        else if (sequence[2] === "J") {
          rows[row] = (rows[row] ?? []).slice(0, column)
          rows.length = row + 1
        } else throw new Error(`the fake screen does not understand ${JSON.stringify(sequence[0])}`)
      } else put(character)
    }
  }

  return {
    write,
    columns: () => columns,
    // What is on the screen, a row at a time, without trailing spaces.
    shown: () => rows.map((line) => line.join("").trimEnd()),
  }
}
