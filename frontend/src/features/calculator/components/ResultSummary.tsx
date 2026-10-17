import { formatTHB } from "@/lib/money";
import type { Quote } from "../api";
import { swatchFor } from "../swatches";
import { bundleNoun, formatPercent, type PromotionView } from "../promotions";

type Props = {
  quote: Quote;
  /** Live rules, used only to word each line. Optional: lines fall back to plain text. */
  promotions?: PromotionView;
  onRecalculate: () => void;
};

export function ResultSummary({ quote, promotions, onRecalculate }: Props) {
  const saved = quote.totalBeforeSatang - quote.finalTotalSatang;
  const itemDiscountTotal = quote.itemDiscounts.reduce(
    (sum, d) => sum + d.amountSatang,
    0,
  );
  const afterPairs = quote.totalBeforeSatang - itemDiscountTotal;
  const memberPercentBp = promotions?.memberPercentBp ?? null;

  return (
    <div className="flex flex-col">
      <h2 className="mt-4 text-2xl font-semibold tracking-tight">Your total</h2>
      <p className="mt-1 text-ink-soft">
        Every line below adds up to the final price.
      </p>

      <div className="mt-6 rounded-2xl bg-ink p-5 text-white">
        <p className="text-white/75">Final total</p>
        <p className="mt-1 text-5xl font-semibold tracking-tight tabular-nums">
          {formatTHB(quote.finalTotalSatang)}
        </p>
        {saved > 0 && (
          <p className="mt-2 text-sm text-white/75">
            You saved {formatTHB(saved)}
          </p>
        )}
      </div>

      <section
        role="region"
        aria-label="Your order"
        className="mt-3 border-b border-line py-4"
      >
        <h3 className="text-sm font-medium text-ink-soft">Your order</h3>
        <ul className="mt-3 flex flex-col gap-3.5">
          {quote.items.map((line) => (
            <li key={line.productId} className="flex items-center gap-3">
              <span
                aria-hidden="true"
                className="size-3 shrink-0 rounded-[4px] ring-1 ring-inset ring-black/10"
                style={{ backgroundColor: swatchFor(line.productId) }}
              />
              <div className="flex-1">
                <p className="leading-tight">{line.name}</p>
                <p className="text-sm text-ink-soft tabular-nums">
                  {line.quantity} × {formatTHB(line.unitPriceSatang)}
                </p>
              </div>
              <span className="font-medium tabular-nums">
                {formatTHB(line.lineTotalSatang)}
              </span>
            </li>
          ))}
        </ul>
      </section>

      <div className="flex flex-col">
        <div className="flex items-baseline justify-between gap-4 border-b border-line py-4">
          <span>Total (before discount)</span>
          <span className="font-medium tabular-nums">
            {formatTHB(quote.totalBeforeSatang)}
          </span>
        </div>

        <section
          role="region"
          aria-label="Discounted items"
          className="border-b border-line py-4"
        >
          <h3 className="text-sm font-medium text-ink-soft">
            Discounted Items
          </h3>
          {quote.itemDiscounts.length === 0 ? (
            <p className="mt-2 text-ink-soft">No item discounts</p>
          ) : (
            <ul className="mt-3 flex flex-col gap-3.5">
              {quote.itemDiscounts.map((d) => {
                const rule = promotions?.bundleRules[d.ruleCode];
                const detail = rule
                  ? `${d.bundles} ${bundleNoun(rule.bundleSize, d.bundles)} · ${formatPercent(rule.percentBp)} off`
                  : null;
                return (
                  <li
                    key={`${d.ruleCode}-${d.productId}`}
                    className="flex items-center gap-3"
                  >
                    <span
                      aria-hidden="true"
                      className="size-3 shrink-0 rounded-[4px] ring-1 ring-inset ring-black/10"
                      style={{ backgroundColor: swatchFor(d.productId) }}
                    />
                    <div className="flex-1">
                      <p className="leading-tight">{d.name}</p>
                      {detail && (
                        <p className="text-sm text-ink-soft tabular-nums">
                          {detail}
                        </p>
                      )}
                    </div>
                    <span className="font-medium tabular-nums">
                      -{formatTHB(d.amountSatang)}
                    </span>
                  </li>
                );
              })}
            </ul>
          )}
        </section>

        <div className="flex items-baseline justify-between gap-4 py-4">
          <div>
            <p>Discount from member card</p>
            {memberPercentBp !== null && quote.memberDiscountSatang > 0 && (
              <p className="text-sm text-ink-soft tabular-nums">
                {formatPercent(memberPercentBp)} of {formatTHB(afterPairs)}
              </p>
            )}
          </div>
          <span className="font-medium tabular-nums">
            -{formatTHB(quote.memberDiscountSatang)}
          </span>
        </div>
      </div>

      <div className="sticky bottom-0 -mx-4 mt-4 border-t border-line bg-white px-4 pt-3 pb-[max(1rem,env(safe-area-inset-bottom))]">
        <button
          type="button"
          onClick={onRecalculate}
          className="h-[52px] w-full rounded-full border border-ink font-medium transition-colors duration-150 hover:bg-field focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent"
        >
          Recalculate
        </button>
      </div>
    </div>
  );
}
