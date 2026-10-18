import { formatTHB } from "@/lib/money";
import type { Product } from "../api";
import { swatchFor } from "../swatches";
import { ProductImage } from "./ProductImage";

type Props = {
  product: Product;
  quantity: number;
  /** Live promotion text for this product, e.g. "5% off each pair". */
  promoLabel?: string;
  onIncrement: () => void;
  onDecrement: () => void;
};

const circle =
  "flex size-11 shrink-0 items-center justify-center rounded-full text-xl leading-none transition-colors duration-150 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent";
const minus = `${circle} bg-field text-ink hover:bg-line active:bg-line`;
const plus = `${circle} bg-ink text-white hover:bg-ink/85 active:bg-ink/85`;

export function ProductRow({
  product,
  quantity,
  promoLabel,
  onIncrement,
  onDecrement,
}: Props) {
  return (
    <li
      className={`flex items-center gap-4 rounded-3xl bg-white p-3 pr-4 transition-all duration-200 ${
        quantity > 0
          ? "shadow-[0_0_0_1px_#F3D8DF,0_8px_24px_-12px_rgba(190,70,100,0.18)]"
          : "shadow-[0_2px_8px_rgba(40,25,25,0.04),0_8px_24px_-16px_rgba(40,25,25,0.12)]"
      }`}
    >
      <ProductImage
        src={product.imageUrl}
        className="size-[72px] shrink-0 rounded-2xl bg-field object-cover"
      />
      <div className="min-w-0 flex-1">
        <p className="font-medium leading-tight">{product.name}</p>
        <p className="mt-1 flex items-center gap-1.5 text-sm text-ink-soft tabular-nums">
          <span
            aria-hidden="true"
            className="size-2.5 shrink-0 rounded-full ring-1 ring-inset ring-black/10"
            style={{ backgroundColor: swatchFor(product.id) }}
          />
          <span>{formatTHB(product.priceSatang)}</span>
        </p>
        {promoLabel && (
          <p className="mt-0.5 text-balance text-sm text-ink-soft">
            {promoLabel}
          </p>
        )}
      </div>
      {quantity === 0 ? (
        <button
          type="button"
          className={plus}
          aria-label={`Increase ${product.name}`}
          onClick={onIncrement}
        >
          +
        </button>
      ) : (
        <div className="flex items-center gap-2">
          <button
            type="button"
            className={minus}
            aria-label={`Decrease ${product.name}`}
            onClick={onDecrement}
          >
            −
          </button>
          <output
            aria-label={`${product.name} quantity`}
            className="min-w-6 text-center text-lg font-semibold tabular-nums"
          >
            {quantity}
          </output>
          <button
            type="button"
            className={plus}
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
