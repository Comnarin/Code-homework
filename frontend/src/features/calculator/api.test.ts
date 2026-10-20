import { describe, expect, it, vi } from "vitest";
import {
  calculateOrder,
  fetchProducts,
  fetchPromotions,
  resetLimits,
} from "./api";

const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });

const validQuote = {
  items: [
    {
      productId: "orange",
      name: "Orange set",
      quantity: 2,
      unitPriceSatang: 12000,
      lineTotalSatang: 24000,
    },
  ],
  totalBeforeSatang: 24000,
  itemDiscounts: [
    {
      ruleCode: "PAIR_5",
      productId: "orange",
      name: "Orange set",
      bundles: 1,
      amountSatang: 1200,
    },
  ],
  memberDiscountSatang: 2280,
  memberBaseSatang: 22800,
  finalTotalSatang: 20520,
};

describe("calculateOrder", () => {
  it("posts the order and returns the parsed quote", async () => {
    const fetchFn = vi.fn().mockResolvedValue(json(validQuote));

    const result = await calculateOrder(
      { memberCard: "MEM0001", items: [{ productId: "orange", quantity: 2 }] },
      fetchFn,
    );

    expect(result).toEqual({ ok: true, quote: validQuote });
    const [url, init] = fetchFn.mock.calls[0]!;
    expect(url).toBe("/api/calculate");
    expect(init.method).toBe("POST");
    expect(JSON.parse(init.body)).toEqual({
      memberCard: "MEM0001",
      items: [{ productId: "orange", quantity: 2 }],
    });
  });

  it("sends the per-session client key so the Red holder can price again", async () => {
    const fetchFn = vi.fn().mockResolvedValue(json(validQuote));

    await calculateOrder(
      { items: [{ productId: "red", quantity: 1 }] },
      fetchFn,
      () => "browser-1",
    );

    const [, init] = fetchFn.mock.calls[0]!;
    expect(init.headers["X-Client-Key"]).toBe("browser-1");
    expect(JSON.parse(init.body)).not.toHaveProperty("clientKey");
  });

  it("omits the client key header when there is none", async () => {
    const fetchFn = vi.fn().mockResolvedValue(json(validQuote));

    await calculateOrder(
      { items: [{ productId: "red", quantity: 1 }] },
      fetchFn,
      () => null,
    );

    const [, init] = fetchFn.mock.calls[0]!;
    expect(init.headers).not.toHaveProperty("X-Client-Key");
  });

  it("returns the server's error code and message", async () => {
    const fetchFn = vi.fn().mockResolvedValue(
      json(
        {
          error: {
            code: "LIMITED_PRODUCT_UNAVAILABLE",
            message: "Red set is reserved by another customer right now.",
          },
        },
        409,
      ),
    );

    const result = await calculateOrder(
      { items: [{ productId: "red", quantity: 1 }] },
      fetchFn,
    );

    expect(result).toEqual({
      ok: false,
      code: "LIMITED_PRODUCT_UNAVAILABLE",
      message: "Red set is reserved by another customer right now.",
    });
  });

  it("reports a network failure as a retryable message", async () => {
    const fetchFn = vi.fn().mockRejectedValue(new TypeError("Failed to fetch"));

    const result = await calculateOrder(
      { items: [{ productId: "blue", quantity: 1 }] },
      fetchFn,
    );

    expect(result).toMatchObject({ ok: false, code: "NETWORK" });
    expect(result.ok === false && result.message).toMatch(/try again/i);
  });

  it("rejects a response that does not match the schema", async () => {
    const fetchFn = vi
      .fn()
      .mockResolvedValue(json({ finalTotalSatang: "lots" }));

    const result = await calculateOrder(
      { items: [{ productId: "blue", quantity: 1 }] },
      fetchFn,
    );

    expect(result).toMatchObject({ ok: false, code: "BAD_RESPONSE" });
  });

  it("handles a non-JSON error page", async () => {
    const fetchFn = vi
      .fn()
      .mockResolvedValue(
        new Response("<html>Bad gateway</html>", { status: 502 }),
      );

    const result = await calculateOrder(
      { items: [{ productId: "blue", quantity: 1 }] },
      fetchFn,
    );

    expect(result).toMatchObject({ ok: false, code: "HTTP_502" });
  });
});

describe("fetchProducts", () => {
  it("returns validated products", async () => {
    const fetchFn = vi.fn().mockResolvedValue(
      json([
        {
          id: "red",
          name: "Red set",
          priceSatang: 5000,
          imageUrl: "/api/products/red/image",
        },
      ]),
    );

    const products = await fetchProducts("http://api:8080", fetchFn);

    expect(products).toEqual([
      {
        id: "red",
        name: "Red set",
        priceSatang: 5000,
        imageUrl: "/api/products/red/image",
      },
    ]);
    expect(fetchFn.mock.calls[0]![0]).toBe("http://api:8080/api/products");
  });

  it("throws when the API fails, so the error boundary can render", async () => {
    const fetchFn = vi.fn().mockResolvedValue(json({}, 500));

    await expect(fetchProducts("http://api:8080", fetchFn)).rejects.toThrow(
      /products/i,
    );
  });

  it("throws when the payload has the wrong shape", async () => {
    const fetchFn = vi.fn().mockResolvedValue(json([{ id: 1 }]));

    await expect(fetchProducts("http://api:8080", fetchFn)).rejects.toThrow();
  });
});

describe("fetchPromotions", () => {
  it("parses the live rules", async () => {
    const rules = [
      {
        code: "PAIR_5",
        type: "BUNDLE_PERCENT",
        percent: 5,
        bundleSize: 2,
        productIds: ["yellow"],
      },
      { code: "MEMBER_10", type: "MEMBER_PERCENT", percent: 10 },
    ];
    const fetchFn = vi.fn().mockResolvedValue(json(rules));

    expect(await fetchPromotions("http://api", fetchFn)).toEqual(rules);
    expect(fetchFn).toHaveBeenCalledWith("http://api/api/promotions");
  });

  it("throws on a server error", async () => {
    const fetchFn = vi.fn().mockResolvedValue(json({}, 500));
    await expect(fetchPromotions("http://api", fetchFn)).rejects.toThrow();
  });
});

describe("resetLimits", () => {
  it("posts to the test endpoint", async () => {
    const fetchFn = vi
      .fn()
      .mockResolvedValue(new Response(null, { status: 204 }));

    expect(await resetLimits(fetchFn)).toEqual({ ok: true });
    expect(fetchFn).toHaveBeenCalledWith("/api/test/reset-limits", {
      method: "POST",
    });
  });

  it("reports a rejected reset and a network failure", async () => {
    const notFound = vi.fn().mockResolvedValue(json({}, 404));
    expect((await resetLimits(notFound)).ok).toBe(false);

    const down = vi.fn().mockRejectedValue(new TypeError("fetch failed"));
    expect((await resetLimits(down)).ok).toBe(false);
  });
});
