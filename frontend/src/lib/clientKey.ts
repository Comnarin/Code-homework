const STORAGE_KEY = "food-store-client-key";

/**
 * A random key that stays the same for this browser tab session. The API uses
 * it to tell that a repeat calculate comes from whoever already holds the Red
 * set, so they can price again while everyone else is still blocked. It is not
 * an account or a secret. Returns null when no key can be made or kept.
 */
export function getClientKey(
  storage: Pick<Storage, "getItem" | "setItem"> | null = safeSessionStorage(),
): string | null {
  try {
    const existing = storage?.getItem(STORAGE_KEY);
    if (existing) return existing;
    const created = crypto.randomUUID();
    storage?.setItem(STORAGE_KEY, created);
    return created;
  } catch {
    return null;
  }
}

function safeSessionStorage(): Storage | null {
  try {
    return typeof window === "undefined" ? null : window.sessionStorage;
  } catch {
    return null;
  }
}
