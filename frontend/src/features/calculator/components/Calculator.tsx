"use client";

import { calculateOrder, type CalculateFn, type Product } from "../api";
import { hasItems } from "../state";
import { useCalculator } from "../useCalculator";
import { ProductRow } from "./ProductRow";
import { ResultSummary } from "./ResultSummary";

type Props = { products: Product[]; calculate?: CalculateFn };

export function Calculator({ products, calculate = calculateOrder }: Props) {
  const { state, increment, decrement, setMemberCard, submit, recalculate } =
    useCalculator(calculate);

  if (state.status === "result" && state.quote) {
    return <ResultSummary quote={state.quote} onRecalculate={recalculate} />;
  }

  const loading = state.status === "loading";
  const count = Object.values(state.quantities).reduce((sum, q) => sum + q, 0);

  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        void submit();
      }}
    >
      {/* A disabled fieldset locks every control while a request is in flight, so the quote always matches what was sent. */}
      <fieldset
        disabled={loading}
        className="flex min-w-0 flex-col gap-8 border-0 p-0"
      >
        <p className="-mb-4 text-ink-soft">
          Choose your sets to see the price, with any discounts.
        </p>
        <ul>
          {products.map((p) => (
            <ProductRow
              key={p.id}
              product={p}
              quantity={state.quantities[p.id] ?? 0}
              onIncrement={() => increment(p.id)}
              onDecrement={() => decrement(p.id)}
            />
          ))}
        </ul>

        <div className="flex flex-col gap-2">
          <label htmlFor="member-card" className="font-medium">
            Card Number
          </label>
          <input
            id="member-card"
            value={state.memberCard}
            onChange={(e) => setMemberCard(e.target.value)}
            autoComplete="off"
            aria-describedby="member-card-hint"
            className="h-12 rounded-xl border border-line bg-field px-4 transition-colors duration-150 focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-accent disabled:text-ink-soft"
          />
          <p id="member-card-hint" className="text-sm text-ink-soft">
            Optional. Members get 10% off.
          </p>
        </div>

        <div className="sticky bottom-0 -mx-4 flex flex-col gap-3 border-t border-line bg-white px-4 pt-3 pb-[max(1rem,env(safe-area-inset-bottom))]">
          {state.errorMessage && (
            <p
              role="alert"
              className="rounded-xl border border-accent/30 bg-accent-soft p-4 text-sm"
            >
              {state.errorMessage}
            </p>
          )}
          <div className="flex items-center gap-4">
            <p className="min-w-16 text-sm tabular-nums text-ink-soft">
              {count === 0
                ? "No sets yet"
                : `${count} ${count === 1 ? "set" : "sets"}`}
            </p>
            <button
              type="submit"
              disabled={!hasItems(state) || loading}
              className="h-12 flex-1 rounded-full bg-ink font-medium text-white transition-colors duration-150 hover:bg-ink/90 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent disabled:cursor-not-allowed disabled:bg-line disabled:text-ink-soft"
            >
              {loading ? "Calculating…" : "Calculate"}
            </button>
          </div>
        </div>
      </fieldset>
    </form>
  );
}
