import { describe, expect, it } from "vitest";
import {
  defaultChatTextSize,
  isChatTextSize,
  resolveChatTextSize,
} from "./textSize";

describe("isChatTextSize", () => {
  it("accepts every supported setting", () => {
    expect(["compact", "default", "large"].every(isChatTextSize)).toBe(true);
  });

  it("rejects missing and unknown persisted values", () => {
    expect(isChatTextSize(null)).toBe(false);
    expect(isChatTextSize("medium")).toBe(false);
  });

  it("defaults missing and unknown persisted values to 15px", () => {
    expect(defaultChatTextSize).toBe("default");
    expect(resolveChatTextSize(null)).toBe("default");
    expect(resolveChatTextSize("medium")).toBe("default");
  });
});
