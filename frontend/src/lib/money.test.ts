import { describe, expect, it } from "vitest";
import { formatTHB } from "./money";

describe("formatTHB", () => {
  it("formats satang as baht with two decimals", () => {
    expect(formatTHB(22800)).toMatch(/228\.00/);
    expect(formatTHB(22800)).toContain("฿");
  });

  it("adds thousands separators", () => {
    expect(formatTHB(123456700)).toMatch(/1,234,567\.00/);
  });

  it("formats zero", () => {
    expect(formatTHB(0)).toMatch(/0\.00/);
  });

  it("keeps satang precision", () => {
    expect(formatTHB(20520)).toMatch(/205\.20/);
    expect(formatTHB(1)).toMatch(/0\.01/);
  });
});
