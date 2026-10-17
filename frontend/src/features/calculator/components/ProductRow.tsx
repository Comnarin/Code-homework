import { formatTHB } from "@/lib/money";
import type { Product } from "../api";
import { swatchFor } from "../swatches";

type Props = {
  product: Product;
  quantity: number;
  /** Live promotion text for this product, e.g. "5% off each pair". */
  promoLabel?: string;
  onIncrement: () => void;
  onDecrement: () => void;
};

const step =
  "flex size-11 items-center justify-center rounded-full text-xl leading-none text-ink transition-colors duration-150 hover:bg-line active:bg-line focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-accent disabled:cursor-not-allowed disabled:text-ink-soft/40 disabled:hover:bg-transparent";

export function ProductRow({
  product,
  quantity,
  promoLabel,
  onIncrement,
  onDecrement,
}: Props) {
  return (
    <li
      className={`flex items-center gap-3.5 rounded-2xl px-3 py-2.5 transition-colors duration-150 ${quantity > 0 ? "bg-field" : ""}`}
    >
      <span
        aria-hidden="true"
        className="size-11 shrink-0 rounded-xl ring-1 ring-inset ring-black/10"
        style={{ backgroundColor: swatchFor(product.id) }}
      />
      <div className="min-w-0 flex-1">
        <p className="font-medium leading-tight">{product.name}</p>
        <p className="mt-0.5 text-sm text-ink-soft tabular-nums">
          {formatTHB(product.priceSatang)}
          {promoLabel && ` · ${promoLabel}`}
        </p>
      </div>
      {quantity === 0 ? (
        <button
          type="button"
          className={`${step} border border-line bg-white`}
          aria-label={`Increase ${product.name}`}
          onClick={onIncrement}
        >
          +
        </button>
      ) : (
        <div className="flex items-center rounded-full border border-line bg-white">
          <button
            type="button"
            className={step}
            aria-label={`Decrease ${product.name}`}
            onClick={onDecrement}
          >
            −
          </button>
          <output
            aria-label={`${product.name} quantity`}
            className="min-w-5 text-center font-semibold tabular-nums"
          >
            {quantity}
          </output>
          <button
            type="button"
            className={step}
            aria-label={`Increase ${product.name}`}
            onClick={onIncrement}
          >
            +
          </button>
        </div>
      )}
    </li>
  );
}
