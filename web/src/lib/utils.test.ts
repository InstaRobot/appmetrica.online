import { describe, expect, it } from "vitest";
import { cn } from "./utils";

describe("cn", () => {
  it("merges conditional classes and resolves Tailwind conflicts", () => {
    const hidden = false;
    expect(cn("px-2 py-1", hidden && "hidden", "px-4")).toBe("py-1 px-4");
  });
});
