import { describe, expect, it } from "vitest";
import { readApprovalArgs } from "./approvalArgs";

describe("readApprovalArgs", () => {
  it("reads edit_file into a diff", () => {
    const view = readApprovalArgs(
      "edit_file",
      JSON.stringify({
        path: "/tmp/a.yaml",
        old_string: "ramp: none",
        new_string: "ramp: { minutes: 90 }",
      }),
    );
    expect(view).toEqual({
      shape: "diff",
      subject: "/tmp/a.yaml",
      oldText: "ramp: none",
      newText: "ramp: { minutes: 90 }",
    });
  });

  it("reads edit_db into a statement", () => {
    const view = readApprovalArgs(
      "edit_db",
      JSON.stringify({ db: "finance", statement: "DELETE FROM x WHERE id=1" }),
    );
    expect(view).toEqual({
      shape: "statement",
      subject: "finance",
      statement: "DELETE FROM x WHERE id=1",
    });
  });

  it("falls back to JSON for an unknown gated tool", () => {
    const view = readApprovalArgs("some_future_tool", '{"a":1}');
    expect(view.shape).toBe("json");
    expect(view.shape === "json" && view.json).toContain('"a": 1');
  });

  it("falls back to JSON when edit_file args are incomplete", () => {
    // A malformed call must stay reviewable rather than render an empty diff.
    const view = readApprovalArgs("edit_file", '{"path":"/tmp/a"}');
    expect(view.shape).toBe("json");
  });

  it("shows unparseable args verbatim rather than hiding them", () => {
    const view = readApprovalArgs("edit_file", "{not json");
    expect(view).toEqual({ shape: "json", subject: "", json: "{not json" });
  });
});
