import { formatTHB } from "@/lib/money";
import type { Quote } from "../api";

type Props = { quote: Quote; onRecalculate: () => void };

const card = "rounded-2xl bg-white p-4 shadow-sm";

export function ResultSummary({ quote, onRecalculate }: Props) {
  return (
    <div className="flex flex-col gap-4">
      <h2 className="text-center text-2xl font-bold">Calculate Total</h2>

      <section className={card}>
        <p className="text-lg">Total (before discount)</p>
        <p className="font-bold">{formatTHB(quote.totalBeforeSatang)}</p>
      </section>

      <section className={card} role="region" aria-label="Discounted items">
        <h3 className="text-lg">Discounted Items</h3>
        {quote.itemDiscounts.length === 0 ? (
          <p className="text-sm text-slate-600">No item discounts</p>
        ) : (
          <ul>
            {quote.itemDiscounts.map((d) => (
              <li key={`${d.ruleCode}-${d.productId}`} className="flex justify-between text-sm">
                <span>{d.name}</span>
                <span>-{formatTHB(d.amountSatang)}</span>
              </li>
            ))}
          </ul>
        )}
      </section>

      <section className={card}>
        <p className="text-lg">Discount from member card</p>
        <p className="font-bold">-{formatTHB(quote.memberDiscountSatang)}</p>
      </section>

      <section className={card}>
        <p className="text-lg font-bold">Final total</p>
        <p className="font-bold">{formatTHB(quote.finalTotalSatang)}</p>
      </section>

      <button
        type="button"
        onClick={onRecalculate}
        className="mx-auto rounded-full bg-sky-500 px-6 py-2 text-white transition hover:bg-sky-600 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-sky-700"
      >
        Recalculate
      </button>
    </div>
  );
}
