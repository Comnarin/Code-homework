import { cacheLife } from "next/cache";
import { API_URL } from "@/lib/env";
import {
  fetchProducts,
  fetchPromotions,
  type Product,
  type Promotion,
} from "./api";

/** Server-only. The menu changes rarely, so it is cached at runtime (Cache Components). */
export async function getProducts(): Promise<Product[]> {
  "use cache";
  cacheLife("minutes");
  return fetchProducts(API_URL);
}

/**
 * Server-only. Not cached, so a rule change shows on the next page load.
 * Promotions only decorate the screen (badges, hints); the API still prices
 * the order, so a failure here must not block ordering.
 */
export async function getPromotions(): Promise<Promotion[]> {
  try {
    return await fetchPromotions(API_URL);
  } catch {
    return [];
  }
}
