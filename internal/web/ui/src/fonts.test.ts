/// <reference types="node" />
import { existsSync, readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const ui = join(dirname(fileURLToPath(import.meta.url)), "..");
const read = (path: string) => readFileSync(join(ui, path), "utf8");
const fonts = read("public/fonts/fonts.css");

describe("bundled fonts", () => {
  it("loads no stylesheet or font from another host", () => {
    for (const file of ["src/theme.css", "public/fonts/fonts.css"]) {
      expect(read(file), file).not.toMatch(/(@import|url\()\s*["']?(https?:)?\/\//);
    }
    expect(read("index.html")).not.toMatch(/href="(https?:)?\/\//);
  });

  it("is loaded by the page, not by the stylesheet Workspace tabs share", () => {
    expect(read("index.html")).toContain('<link rel="stylesheet" href="/fonts/fonts.css" />');
    expect(read("src/theme.css")).not.toMatch(/@font-face|fonts\.css/);
  });

  it("points every font face at a file that ships with the app", () => {
    const files = [...fonts.matchAll(/url\('\.\/([^']+)'\)/g)].map((m) => m[1]);
    expect(files.length).toBeGreaterThan(0);
    for (const file of files) expect(existsSync(join(ui, "public/fonts", file)), file).toBe(true);
  });

  it("covers every family the design uses", () => {
    const families = new Set([...fonts.matchAll(/font-family: '([^']+)'/g)].map((m) => m[1]));
    expect([...families].sort()).toEqual(["Caveat", "IBM Plex Mono", "IBM Plex Sans"]);
  });
});
