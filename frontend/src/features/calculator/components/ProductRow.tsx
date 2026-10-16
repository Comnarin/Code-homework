import { formatTHB } from "@/lib/money";
import type { Product } from "../api";
import { swatchFor } from "../swatches";

type Props = {
  product: Product;
  quantity: number;
  onIncrement: () => void;
  onDecrement: () => void;
};

const step =
  "flex size-11 items-center justify-center rounded-full text-xl leading-none text-ink transition-colors duration-150 hover:bg-line active:bg-line focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-accent disabled:cursor-not-allowed disabled:text-ink-soft/40 disabled:hover:bg-transparent";

export function ProductRow({
  product,
  quantity,
  onIncrement,
  onDecrement,
}: Props) {
  return (
    <li className="flex items-center gap-3 border-b border-line py-3 last:border-b-0">
      <span
        aria-hidden="true"
        className="size-4 shrink-0 rounded-full ring-1 ring-inset ring-black/10"
        style={{ backgroundColor: swatchFor(product.id) }}
      />
      <div className="min-w-0 flex-1">
        <p className="font-medium leading-tight">{product.name}</p>
        <p className="mt-0.5 text-sm text-ink-soft tabular-nums">
          {formatTHB(product.priceSatang)}
        </p>
      </div>
      <div className="flex items-center rounded-full border border-line bg-field">
        <button
          type="button"
          className={step}
          aria-label={`Decrease ${product.name}`}
          disabled={quantity === 0}
          onClick={onDecrement}
        >
          −
        </button>
        <output
          aria-label={`${product.name} quantity`}
          className={`min-w-7 text-center font-medium tabular-nums ${quantity === 0 ? "text-ink-soft" : "text-ink"}`}
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
    </li>
  );
}
