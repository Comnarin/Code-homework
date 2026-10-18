"use client";

import { useEffect, useRef, useState } from "react";
import {
  calculateOrder,
  resetLimits,
  type CalculateFn,
  type Product,
  type Promotion,
  type ResetFn,
} from "../api";
import { buildPromotionView, formatPercent } from "../promotions";
import { hasItems } from "../state";
import { useCalculator } from "../useCalculator";
import { ProductRow } from "./ProductRow";
import { ResultSummary } from "./ResultSummary";

type Props = {
  products: Product[];
  promotions?: Promotion[];
  calculate?: CalculateFn;
  /** Shows the test-only reset control. Off in production. */
  showTestTools?: boolean;
  reset?: ResetFn;
};

export function Calculator({
  products,
  promotions = [],
  calculate = calculateOrder,
  showTestTools = false,
  reset = resetLimits,
}: Props) {
  const { state, increment, decrement, setMemberCard, submit, recalculate } =
    useCalculator(calculate);
  const view = buildPromotionView(promotions);

  // After Recalculate the result disappears, so focus goes to the first
  // control of the form rather than being lost.
  const formRef = useRef<HTMLFormElement>(null);
  const returnFocus = useRef(false);
  const showingResult = state.status === "result" && !!state.quote;
  useEffect(() => {
    if (showingResult || !returnFocus.current) return;
    returnFocus.current = false;
    formRef.current
      ?.querySelector<HTMLElement>(
        "button:not(:disabled), input:not(:disabled)",
      )
      ?.focus();
  }, [showingResult]);

  if (state.status === "result" && state.quote) {
    return (
      <ResultSummary
        quote={state.quote}
        promotions={view}
        onRecalculate={() => {
          returnFocus.current = true;
          recalculate();
        }}
      />
    );
  }

  const loading = state.status === "loading";
  const count = Object.values(state.quantities).reduce((sum, q) => sum + q, 0);

  return (
    <form
      ref={formRef}
      onSubmit={(e) => {
        e.preventDefault();
        void submit();
      }}
    >
      {/* A disabled fieldset locks every control while a request is in flight, so the quote always matches what was sent. */}
      <fieldset
        disabled={loading}
        className="flex min-w-0 flex-col gap-6 border-0 p-0"
      >
        <p className="-mb-2 text-ink-soft">
          Pick your sets and see the price, discounts included.
        </p>
        <ul className="flex flex-col gap-3">
          {products.map((p) => (
            <ProductRow
              key={p.id}
              product={p}
              quantity={state.quantities[p.id] ?? 0}
              promoLabel={view.productLabels[p.id]}
              onIncrement={() => increment(p.id)}
              onDecrement={() => decrement(p.id)}
            />
          ))}
        </ul>

        <div className="flex flex-col gap-2">
          <label htmlFor="member-card" className="font-medium">
            Member card
          </label>
          <input
            id="member-card"
            value={state.memberCard}
            onChange={(e) => setMemberCard(e.target.value)}
            autoComplete="off"
            aria-describedby="member-card-hint"
            className="h-12 rounded-xl border border-line bg-white px-4 transition-colors duration-150 focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-accent disabled:text-ink-soft"
          />
          <p id="member-card-hint" className="text-sm text-ink-soft">
            {view.memberPercent === null
              ? "Optional."
              : `Optional. Members get ${formatPercent(view.memberPercent)} off after pair discounts.`}
          </p>
        </div>

        {showTestTools && <TestTools reset={reset} />}

        <div className="sticky bottom-0 -mx-4 flex flex-col gap-3 border-t border-line bg-white px-4 pt-3 pb-[max(1rem,env(safe-area-inset-bottom))]">
          {state.errorMessage && (
            <p
              role="alert"
              className="rounded-xl bg-notice p-3.5 text-sm text-notice-ink"
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
              className="h-[52px] flex-1 rounded-full bg-ink font-medium text-white transition-colors duration-150 hover:bg-ink/90 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent disabled:cursor-not-allowed disabled:bg-line disabled:text-ink-soft"
            >
              {loading ? "Calculating…" : "Calculate"}
            </button>
          </div>
        </div>
      </fieldset>
    </form>
  );
}

/** Test aid: frees the Red set so the one-per-hour rule can be tried again. */
function TestTools({ reset }: { reset: ResetFn }) {
  const [message, setMessage] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function onReset() {
    setBusy(true);
    const result = await reset();
    setMessage(
      result.ok
        ? "Red set limit cleared. It can be ordered again."
        : result.message,
    );
    setBusy(false);
  }

  return (
    <div className="flex flex-col gap-2 rounded-xl bg-field p-4">
      <p className="text-sm font-medium">Test tools</p>
      <button
        type="button"
        onClick={() => void onReset()}
        disabled={busy}
        className="h-11 rounded-full border border-ink bg-white text-sm font-medium transition-colors duration-150 hover:bg-line focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent disabled:cursor-not-allowed disabled:text-ink-soft"
      >
        {busy ? "Resetting…" : "Reset Red set limit"}
      </button>
      {message && (
        <p role="status" className="text-sm text-ink-soft">
          {message}
        </p>
      )}
    </div>
  );
}
