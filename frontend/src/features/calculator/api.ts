import { z } from "zod";

const productSchema = z.object({
  id: z.string(),
  name: z.string(),
  priceSatang: z.number().int(),
});

const promotionSchema = z.object({
  code: z.string(),
  type: z.string(),
  percentBp: z.number().int(),
  bundleSize: z.number().int().optional(),
  productIds: z.array(z.string()).optional(),
});

const quoteSchema = z.object({
  items: z.array(
    z.object({
      productId: z.string(),
      name: z.string(),
      quantity: z.number().int(),
      unitPriceSatang: z.number().int(),
      lineTotalSatang: z.number().int(),
    }),
  ),
  totalBeforeSatang: z.number().int(),
  itemDiscounts: z.array(
    z.object({
      ruleCode: z.string(),
      productId: z.string(),
      name: z.string(),
      bundles: z.number().int(),
      amountSatang: z.number().int(),
    }),
  ),
  memberDiscountSatang: z.number().int(),
  finalTotalSatang: z.number().int(),
});

const apiErrorSchema = z.object({
  error: z.object({ code: z.string(), message: z.string() }),
});

export type Product = z.infer<typeof productSchema>;
export type Promotion = z.infer<typeof promotionSchema>;
export type Quote = z.infer<typeof quoteSchema>;
export type CalculateRequest = {
  memberCard?: string;
  items: { productId: string; quantity: number }[];
};
export type CalculateResult =
  { ok: true; quote: Quote } | { ok: false; code: string; message: string };
export type ResetResult = { ok: true } | { ok: false; message: string };
export type ResetFn = () => Promise<ResetResult>;
export type CalculateFn = (req: CalculateRequest) => Promise<CalculateResult>;

/** Called from the browser. Same-origin `/api` is proxied to the Go API, so no CORS setup is needed. */
export async function calculateOrder(
  req: CalculateRequest,
  fetchFn: typeof fetch = fetch,
): Promise<CalculateResult> {
  let res: Response;
  try {
    res = await fetchFn("/api/calculate", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(req),
    });
  } catch {
    return {
      ok: false,
      code: "NETWORK",
      message: "Cannot reach the server. Please try again.",
    };
  }

  const body: unknown = await res.json().catch(() => null);

  if (res.ok) {
    const parsed = quoteSchema.safeParse(body);
    return parsed.success
      ? { ok: true, quote: parsed.data }
      : {
          ok: false,
          code: "BAD_RESPONSE",
          message: "Unexpected response from the server.",
        };
  }

  const apiError = apiErrorSchema.safeParse(body);
  return apiError.success
    ? {
        ok: false,
        code: apiError.data.error.code,
        message: apiError.data.error.message,
      }
    : {
        ok: false,
        code: `HTTP_${res.status}`,
        message: "Something went wrong. Please try again.",
      };
}

/** Called from a Server Component with the internal API address. */
export async function fetchProducts(
  baseUrl: string,
  fetchFn: typeof fetch = fetch,
): Promise<Product[]> {
  const res = await fetchFn(`${baseUrl}/api/products`);
  if (!res.ok) throw new Error(`Could not load products (HTTP ${res.status})`);
  return z.array(productSchema).parse(await res.json());
}

/** Called from a Server Component. The rules are read live so the UI never hard-codes them. */
export async function fetchPromotions(
  baseUrl: string,
  fetchFn: typeof fetch = fetch,
): Promise<Promotion[]> {
  const res = await fetchFn(`${baseUrl}/api/promotions`);
  if (!res.ok)
    throw new Error(`Could not load promotions (HTTP ${res.status})`);
  return z.array(promotionSchema).parse(await res.json());
}

/** Test aid: frees the Red set so the one-per-hour rule can be tried again. */
export async function resetLimits(
  fetchFn: typeof fetch = fetch,
): Promise<ResetResult> {
  try {
    const res = await fetchFn("/api/test/reset-limits", { method: "POST" });
    return res.ok
      ? { ok: true }
      : { ok: false, message: "Could not reset the limit." };
  } catch {
    return {
      ok: false,
      message: "Cannot reach the server. Please try again.",
    };
  }
}
