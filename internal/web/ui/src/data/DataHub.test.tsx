import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { DataHubView } from "./DataHub";

describe("DataHubView", () => {
  it("keeps Database and Memory inside one extensible Data destination", () => {
    const html = renderToStaticMarkup(
      <DataHubView
        source="memory"
        onSource={() => undefined}
        database={<div>physical schema</div>}
        memory={<div>knowledge graph</div>}
      />,
    );

    for (const text of ["Data", "Database", "Memory", "knowledge graph"]) expect(html).toContain(text);
    expect(html).toContain('aria-current="page"');
    expect(html).not.toContain("physical schema");
    expect(html).not.toContain("Tests");
    expect(html).not.toContain("Experiments");
  });
});

it("selects Usage without mounting other Data sources", () => {
  const html = renderToStaticMarkup(<DataHubView source="usage" onSource={() => undefined} database={<div>schema-only</div>} memory={<div>memory-only</div>} usage={<div>token-measurements</div>} />);
  expect(html).toContain("token-measurements");
  expect(html).not.toContain("schema-only");
  expect(html).not.toContain("memory-only");
});
