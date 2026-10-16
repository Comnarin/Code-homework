import { cacheLife } from "next/cache";
import { API_URL } from "@/lib/env";
import { fetchProducts, type Product } from "./api";

/** Server-only. The menu changes rarely, so it is cached at runtime (Cache Components). */
export async function getProducts(): Promise<Product[]> {
  "use cache";
  cacheLife("minutes");
  return fetchProducts(API_URL);
}
