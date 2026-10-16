import { z } from "zod";

const productSchema = z.object({
  id: z.string(),
  name: z.string(),
  priceSatang: z.number().int(),
});

const quoteSchema = z.object({
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

const apiErrorSchema = z.object({ error: z.object({ code: z.string(), message: z.string() }) });

export type Product = z.infer<typeof productSchema>;
export type Quote = z.infer<typeof quoteSchema>;
export type CalculateRequest = { memberCard?: string; items: { productId: string; quantity: number }[] };
export type CalculateResult = { ok: true; quote: Quote } | { ok: false; code: string; message: string };
export type CalculateFn = (req: CalculateRequest) => Promise<CalculateResult>;

/** Called from the browser. Same-origin `/api` is proxied to the Go API, so the `cid` cookie travels with it. */
export async function calculateOrder(req: CalculateRequest, fetchFn: typeof fetch = fetch): Promise<CalculateResult> {
  let res: Response;
  try {
    res = await fetchFn("/api/calculate", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(req),
      credentials: "same-origin",
    });
  } catch {
    return { ok: false, code: "NETWORK", message: "Cannot reach the server. Please try again." };
  }

  const body: unknown = await res.json().catch(() => null);

  if (res.ok) {
    const parsed = quoteSchema.safeParse(body);
    return parsed.success
      ? { ok: true, quote: parsed.data }
      : { ok: false, code: "BAD_RESPONSE", message: "Unexpected response from the server." };
  }

  const apiError = apiErrorSchema.safeParse(body);
  return apiError.success
    ? { ok: false, code: apiError.data.error.code, message: apiError.data.error.message }
    : { ok: false, code: `HTTP_${res.status}`, message: "Something went wrong. Please try again." };
}

/** Called from a Server Component with the internal API address. */
export async function fetchProducts(baseUrl: string, fetchFn: typeof fetch = fetch): Promise<Product[]> {
  const res = await fetchFn(`${baseUrl}/api/products`, { next: { revalidate: 60 } });
  if (!res.ok) throw new Error(`Could not load products (HTTP ${res.status})`);
  return z.array(productSchema).parse(await res.json());
}
