import { formatTHB } from "@/lib/money";
import type { Quote } from "../api";

type Props = { quote: Quote; onRecalculate: () => void };

function Row({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-baseline justify-between gap-4 border-t border-line py-4">
      <span>{label}</span>
      <span className="font-medium tabular-nums">{value}</span>
    </div>
  );
}

export function ResultSummary({ quote, onRecalculate }: Props) {
  const saved = quote.totalBeforeSatang - quote.finalTotalSatang;

  return (
    <div className="flex flex-col">
      <h2 className="mt-4 text-2xl font-semibold tracking-tight">Your total</h2>
      <p className="mt-1 text-ink-soft">Here is how the price adds up.</p>

      <div className="mt-6 flex flex-col">
        <div className="flex items-baseline justify-between gap-4 pb-4">
          <span>Total (before discount)</span>
          <span className="font-medium tabular-nums">
            {formatTHB(quote.totalBeforeSatang)}
          </span>
        </div>

        <section
          role="region"
          aria-label="Discounted items"
          className="border-t border-line py-4"
        >
          <h3 className="text-sm font-medium text-ink-soft">
            Discounted Items
          </h3>
          {quote.itemDiscounts.length === 0 ? (
            <p className="mt-2 text-ink-soft">No item discounts</p>
          ) : (
            <ul className="mt-2 flex flex-col gap-2">
              {quote.itemDiscounts.map((d) => (
                <li
                  key={`${d.ruleCode}-${d.productId}`}
                  className="flex items-baseline justify-between gap-4"
                >
                  <span>{d.name}</span>
                  <span className="font-medium tabular-nums">
                    -{formatTHB(d.amountSatang)}
                  </span>
                </li>
              ))}
            </ul>
          )}
        </section>

        <Row
          label="Discount from member card"
          value={`-${formatTHB(quote.memberDiscountSatang)}`}
        />
      </div>

      <div className="mt-2 rounded-2xl bg-ink p-5 text-white">
        <div className="flex items-baseline justify-between gap-4">
          <span className="text-lg">Final total</span>
          <span className="text-4xl font-semibold tracking-tight tabular-nums">
            {formatTHB(quote.finalTotalSatang)}
          </span>
        </div>
        {saved > 0 && (
          <p className="mt-2 text-sm text-white/75">
            You saved {formatTHB(saved)}
          </p>
        )}
      </div>

      <div className="sticky bottom-0 -mx-4 mt-6 border-t border-line bg-white px-4 pt-3 pb-[max(1rem,env(safe-area-inset-bottom))]">
        <button
          type="button"
          onClick={onRecalculate}
          className="h-12 w-full rounded-full border border-ink font-medium transition-colors duration-150 hover:bg-field focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent"
        >
          Recalculate
        </button>
      </div>
    </div>
  );
}
