import { describe, expect, it } from "vitest";
import { swatchFor } from "./swatches";

describe("swatchFor", () => {
  it("gives each of the seven sets its own colour", () => {
    const ids = ["red", "green", "blue", "yellow", "pink", "purple", "orange"];
    const colours = ids.map(swatchFor);
    expect(new Set(colours).size).toBe(ids.length);
  });

  it("falls back to a neutral for an unknown product", () => {
    expect(swatchFor("brand-new-set")).toBe(swatchFor("another-new-set"));
    expect(swatchFor("brand-new-set")).not.toBe(swatchFor("red"));
  });
});
