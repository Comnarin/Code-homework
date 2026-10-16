"use client";

import { calculateOrder, type CalculateFn, type Product } from "../api";
import { hasItems } from "../state";
import { useCalculator } from "../useCalculator";
import { ProductRow } from "./ProductRow";
import { ResultSummary } from "./ResultSummary";

type Props = { products: Product[]; calculate?: CalculateFn };

export function Calculator({ products, calculate = calculateOrder }: Props) {
  const { state, increment, decrement, setMemberCard, submit, recalculate } = useCalculator(calculate);

  if (state.status === "result" && state.quote) {
    return <ResultSummary quote={state.quote} onRecalculate={recalculate} />;
  }

  const loading = state.status === "loading";
  return (
    <form
      className="flex flex-col gap-4"
      onSubmit={(e) => {
        e.preventDefault();
        void submit();
      }}
    >
      <ul className="flex flex-col gap-3">
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

      <div className="flex flex-col gap-1">
        <label htmlFor="member-card" className="text-sm font-medium">
          Card Number
        </label>
        <input
          id="member-card"
          value={state.memberCard}
          onChange={(e) => setMemberCard(e.target.value)}
          autoComplete="off"
          className="rounded-full bg-white px-4 py-3 shadow-sm focus-visible:outline focus-visible:outline-2 focus-visible:outline-sky-600"
        />
      </div>

      {state.errorMessage && (
        <p role="alert" className="rounded-xl bg-red-50 p-3 text-sm text-red-800">
          {state.errorMessage}
        </p>
      )}

      <button
        type="submit"
        disabled={!hasItems(state) || loading}
        className="rounded-full bg-sky-500 py-3 text-white transition hover:bg-sky-600 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-sky-700 disabled:cursor-not-allowed disabled:bg-sky-200"
      >
        {loading ? "Calculating…" : "Calculate"}
      </button>
    </form>
  );
}
