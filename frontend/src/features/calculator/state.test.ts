import { describe, expect, it } from "vitest";
import type { Quote } from "./api";
import {
  buildRequest,
  calculatorReducer,
  hasItems,
  initialState,
  MAX_QUANTITY,
  type State,
} from "./state";

const quote: Quote = {
  items: [],
  totalBeforeSatang: 100,
  itemDiscounts: [],
  memberDiscountSatang: 0,
  memberBaseSatang: 22800,
  finalTotalSatang: 100,
};
const withQty = (
  quantities: Record<string, number>,
  extra: Partial<State> = {},
): State => ({
  ...initialState,
  quantities,
  ...extra,
});

describe("calculatorReducer", () => {
  it("increments a product from zero", () => {
    const s = calculatorReducer(initialState, {
      type: "increment",
      productId: "pink",
    });
    expect(s.quantities).toEqual({ pink: 1 });
  });

  it("never goes below zero", () => {
    const s = calculatorReducer(initialState, {
      type: "decrement",
      productId: "pink",
    });
    expect(s.quantities.pink ?? 0).toBe(0);
  });

  it("decrements down to zero", () => {
    const s = calculatorReducer(withQty({ pink: 1 }), {
      type: "decrement",
      productId: "pink",
    });
    expect(s.quantities.pink).toBe(0);
  });

  it("caps quantity at the API limit", () => {
    const s = calculatorReducer(withQty({ pink: MAX_QUANTITY }), {
      type: "increment",
      productId: "pink",
    });
    expect(s.quantities.pink).toBe(MAX_QUANTITY);
  });

  it("stores the member card as typed", () => {
    const s = calculatorReducer(initialState, {
      type: "setMemberCard",
      value: " MEM0001",
    });
    expect(s.memberCard).toBe(" MEM0001");
  });

  it("moves idle -> loading -> result", () => {
    const loading = calculatorReducer(initialState, { type: "submit" });
    expect(loading.status).toBe("loading");
    const done = calculatorReducer(loading, { type: "succeeded", quote });
    expect(done).toMatchObject({ status: "result", quote });
  });

  it("keeps the cart and card when a request fails", () => {
    const start = withQty(
      { red: 1 },
      { memberCard: "MEM0001", status: "loading" },
    );
    const s = calculatorReducer(start, {
      type: "failed",
      message: "Red set is reserved",
    });
    expect(s).toMatchObject({
      status: "error",
      errorMessage: "Red set is reserved",
      memberCard: "MEM0001",
    });
    expect(s.quantities).toEqual({ red: 1 });
  });

  it("clears the previous error when submitting again", () => {
    const s = calculatorReducer(
      withQty({ red: 1 }, { status: "error", errorMessage: "boom" }),
      { type: "submit" },
    );
    expect(s).toMatchObject({ status: "loading", errorMessage: null });
  });

  it("recalculate returns to the form and keeps the inputs", () => {
    const s = calculatorReducer(
      withQty({ pink: 2 }, { status: "result", quote, memberCard: "MEM0001" }),
      {
        type: "recalculate",
      },
    );
    expect(s).toMatchObject({
      status: "idle",
      quote: null,
      memberCard: "MEM0001",
    });
    expect(s.quantities).toEqual({ pink: 2 });
  });
});

describe("buildRequest", () => {
  it("sends only products with quantity above zero", () => {
    const req = buildRequest(withQty({ pink: 2, blue: 0, red: 1 }));
    expect(req.items).toEqual([
      { productId: "pink", quantity: 2 },
      { productId: "red", quantity: 1 },
    ]);
  });

  it("trims the card and omits a blank one", () => {
    expect(
      buildRequest(withQty({ pink: 1 }, { memberCard: "  MEM0001 " }))
        .memberCard,
    ).toBe("MEM0001");
    expect(
      "memberCard" in buildRequest(withQty({ pink: 1 }, { memberCard: "   " })),
    ).toBe(false);
  });
});

describe("hasItems", () => {
  it("is false for an empty or all-zero cart", () => {
    expect(hasItems(initialState)).toBe(false);
    expect(hasItems(withQty({ pink: 0 }))).toBe(false);
  });

  it("is true when any product has a quantity", () => {
    expect(hasItems(withQty({ pink: 1 }))).toBe(true);
  });
});
