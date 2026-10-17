import type { Promotion } from "./api";

type BundleRule = { percentBp: number; bundleSize: number };

export type PromotionView = {
  /** Badge text per product, e.g. "5% off each pair". Absent when no rule applies. */
  productLabels: Record<string, string>;
  /** Member discount in basis points, or null when no member rule is active. */
  memberPercentBp: number | null;
  /** Bundle rules by code, so the receipt can word each discounted line. */
  bundleRules: Record<string, BundleRule>;
};

/** 500 -> "5%", 1250 -> "12.5%". */
export function formatPercent(bp: number): string {
  return `${Number((bp / 100).toFixed(2))}%`;
}

/** "pair" for 2, otherwise "3-set bundle"; pluralised by count. */
export function bundleNoun(size: number, count = 1): string {
  const noun = size === 2 ? "pair" : `${size}-set bundle`;
  return count === 1 ? noun : `${noun}s`;
}

/** Turns the API's live rules into display data. Nothing here knows a colour or a percent. */
export function buildPromotionView(promotions: Promotion[]): PromotionView {
  const productLabels: Record<string, string[]> = {};
  const bundleRules: Record<string, BundleRule> = {};
  let memberPercentBp: number | null = null;

  for (const p of promotions) {
    if (p.type === "MEMBER_PERCENT") {
      memberPercentBp = p.percentBp;
    } else if (p.type === "BUNDLE_PERCENT" && p.bundleSize) {
      bundleRules[p.code] = {
        percentBp: p.percentBp,
        bundleSize: p.bundleSize,
      };
      const label = `${formatPercent(p.percentBp)} off each ${bundleNoun(p.bundleSize)}`;
      for (const id of p.productIds ?? []) {
        (productLabels[id] ??= []).push(label);
      }
    }
  }

  return {
    productLabels: Object.fromEntries(
      Object.entries(productLabels).map(([id, labels]) => [
        id,
        labels.join(" · "),
      ]),
    ),
    memberPercentBp,
    bundleRules,
  };
}
