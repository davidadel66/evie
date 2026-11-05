/// <reference types="node" />
import { existsSync, readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const here = dirname(fileURLToPath(import.meta.url));
const read = (path: string) => readFileSync(join(here, path), "utf8");

describe("bundled fonts", () => {
  it("loads no stylesheet or font from another host", () => {
    for (const file of ["../theme.css", "fonts.css"]) {
      expect(read(file)).not.toMatch(/(@import|url\()\s*["']?(https?:)?\/\//);
    }
  });

  it("points every font face at a file that ships with the app", () => {
    const files = [...read("fonts.css").matchAll(/url\('\.\/([^']+)'\)/g)].map((m) => m[1]);
    expect(files.length).toBeGreaterThan(0);
    for (const file of files) expect(existsSync(join(here, file)), file).toBe(true);
  });

  it("covers every family the design uses", () => {
    const families = new Set([...read("fonts.css").matchAll(/font-family: '([^']+)'/g)].map((m) => m[1]));
    expect([...families].sort()).toEqual(["Caveat", "IBM Plex Mono", "IBM Plex Sans"]);
  });
});
