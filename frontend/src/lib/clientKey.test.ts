import { describe, expect, it } from "vitest";
import { getClientKey } from "./clientKey";

function memoryStorage() {
  const data = new Map<string, string>();
  return {
    getItem: (k: string) => data.get(k) ?? null,
    setItem: (k: string, v: string) => void data.set(k, v),
  };
}

describe("getClientKey", () => {
  it("creates a key once and keeps returning the same one", () => {
    const storage = memoryStorage();
    const first = getClientKey(storage);
    expect(first).toMatch(/^[0-9a-f-]{36}$/);
    expect(getClientKey(storage)).toBe(first);
  });

  it("gives different sessions different keys", () => {
    expect(getClientKey(memoryStorage())).not.toBe(
      getClientKey(memoryStorage()),
    );
  });

  it("returns null instead of throwing when storage is blocked", () => {
    const blocked = {
      getItem: () => {
        throw new Error("denied");
      },
      setItem: () => {
        throw new Error("denied");
      },
    };
    expect(getClientKey(blocked)).toBeNull();
  });
});
