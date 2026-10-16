/**
 * Identity colours for the seven sets. UI-only: they never carry meaning on
 * their own (every set also shows its name), and they are the only strong
 * colour in the interface.
 */
const SWATCHES: Record<string, string> = {
  red: "oklch(0.62 0.21 27)",
  green: "oklch(0.68 0.17 150)",
  blue: "oklch(0.6 0.16 250)",
  yellow: "oklch(0.86 0.17 95)",
  pink: "oklch(0.76 0.15 350)",
  purple: "oklch(0.52 0.19 300)",
  orange: "oklch(0.72 0.17 55)",
};

const NEUTRAL = "oklch(0.8 0.01 15)";

export function swatchFor(productId: string): string {
  return SWATCHES[productId] ?? NEUTRAL;
}
