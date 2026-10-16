import { formatTHB } from "@/lib/money";
import type { Product } from "../api";

type Props = {
  product: Product;
  quantity: number;
  onIncrement: () => void;
  onDecrement: () => void;
};

const stepper =
  "flex h-8 w-8 items-center justify-center rounded-full bg-sky-500 text-white transition hover:bg-sky-600 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-sky-700 disabled:cursor-not-allowed disabled:bg-sky-200";

export function ProductRow({ product, quantity, onIncrement, onDecrement }: Props) {
  return (
    <li className="flex items-center justify-between rounded-2xl bg-white p-4 shadow-sm">
      <div>
        <p className="font-medium text-slate-900">{product.name}</p>
        <p className="text-sm text-slate-600">{formatTHB(product.priceSatang)}</p>
      </div>
      <div className="flex items-center gap-3">
        <button type="button" className={stepper} aria-label={`Decrease ${product.name}`} disabled={quantity === 0} onClick={onDecrement}>
          −
        </button>
        <output aria-label={`${product.name} quantity`} className="w-8 text-center tabular-nums">
          {quantity}
        </output>
        <button type="button" className={stepper} aria-label={`Increase ${product.name}`} onClick={onIncrement}>
          +
        </button>
      </div>
    </li>
  );
}
