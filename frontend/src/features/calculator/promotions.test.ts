import { describe, expect, it } from "vitest";
import type { Promotion } from "./api";
import { buildPromotionView, bundleNoun, formatPercent } from "./promotions";

const pair = (productIds: string[], percentBp = 500): Promotion => ({
  code: "PAIR",
  type: "BUNDLE_PERCENT",
  percentBp,
  bundleSize: 2,
  productIds,
});
const member = (percentBp: number): Promotion => ({
  code: "MEMBER",
  type: "MEMBER_PERCENT",
  percentBp,
});

describe("buildPromotionView", () => {
  it("labels exactly the products the rule lists, whichever they are", () => {
    const view = buildPromotionView([pair(["yellow"])]);
    expect(view.productLabels).toEqual({ yellow: "5% off each pair" });
  });

  it("follows the rule's percent and bundle size", () => {
    const view = buildPromotionView([
      { ...pair(["blue"], 1250), bundleSize: 3 },
    ]);
    expect(view.productLabels.blue).toBe("12.5% off each 3-set bundle");
  });

  it("reads the member percent from the rule", () => {
    expect(buildPromotionView([member(1500)]).memberPercentBp).toBe(1500);
    expect(buildPromotionView([]).memberPercentBp).toBeNull();
  });

  it("joins labels when several rules cover one product", () => {
    const view = buildPromotionView([
      pair(["pink"]),
      { ...pair(["pink"], 1000), code: "PAIR_10", bundleSize: 4 },
    ]);
    expect(view.productLabels.pink).toBe(
      "5% off each pair · 10% off each 4-set bundle",
    );
  });

  it("indexes bundle rules by code for the receipt", () => {
    expect(buildPromotionView([pair(["green"])]).bundleRules.PAIR).toEqual({
      percentBp: 500,
      bundleSize: 2,
    });
  });
});

describe("formatting", () => {
  it("formats percents and nouns", () => {
    expect(formatPercent(500)).toBe("5%");
    expect(formatPercent(1250)).toBe("12.5%");
    expect(bundleNoun(2)).toBe("pair");
    expect(bundleNoun(2, 3)).toBe("pairs");
    expect(bundleNoun(3, 2)).toBe("3-set bundles");
  });
});
