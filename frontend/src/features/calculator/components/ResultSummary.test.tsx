import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { Quote } from "../api";
import { buildPromotionView } from "../promotions";
import { ResultSummary } from "./ResultSummary";

const quote: Quote = {
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
  finalTotalSatang: 20520,
};

describe("ResultSummary", () => {
  it("shows total, item discounts, member discount and final total", () => {
    render(<ResultSummary quote={quote} onRecalculate={vi.fn()} />);

    expect(
      screen.getByText("Total (before discount)").parentElement,
    ).toHaveTextContent(/240\.00/);
    const items = screen.getByRole("region", { name: "Discounted items" });
    expect(within(items).getByText("Orange set")).toBeInTheDocument();
    expect(within(items).getByText(/-.*12\.00/)).toBeInTheDocument();
    expect(
      screen.getByText("Discount from member card").parentElement
        ?.parentElement,
    ).toHaveTextContent(/-.*22\.80/);
    expect(screen.getByText("Final total").parentElement).toHaveTextContent(
      /205\.20/,
    );
  });

  it("says so when no item got a discount", () => {
    render(
      <ResultSummary
        quote={{ ...quote, itemDiscounts: [], memberDiscountSatang: 0 }}
        onRecalculate={vi.fn()}
      />,
    );
    expect(screen.getByText(/no item discounts/i)).toBeInTheDocument();
  });

  it("recalculates on click", async () => {
    const onRecalculate = vi.fn();
    render(<ResultSummary quote={quote} onRecalculate={onRecalculate} />);
    await userEvent.click(screen.getByRole("button", { name: "Recalculate" }));
    expect(onRecalculate).toHaveBeenCalledTimes(1);
  });

  it("tells the customer how much they saved", () => {
    render(<ResultSummary quote={quote} onRecalculate={vi.fn()} />);
    // 240.00 before, 205.20 after
    expect(screen.getByText(/you saved/i)).toHaveTextContent(/34\.80/);
  });

  it("does not mention savings when there was no discount", () => {
    render(
      <ResultSummary
        quote={{
          items: [
            {
              productId: "blue",
              name: "Blue set",
              quantity: 2,
              unitPriceSatang: 3000,
              lineTotalSatang: 6000,
            },
          ],
          totalBeforeSatang: 6000,
          itemDiscounts: [],
          memberDiscountSatang: 0,
          finalTotalSatang: 6000,
        }}
        onRecalculate={vi.fn()}
      />,
    );
    expect(screen.queryByText(/you saved/i)).not.toBeInTheDocument();
  });

  it("words each discount line from the live rules", () => {
    const promotions = buildPromotionView([
      {
        code: "PAIR_5",
        type: "BUNDLE_PERCENT",
        percent: 5,
        bundleSize: 2,
        productIds: ["orange"],
      },
      { code: "MEMBER_10", type: "MEMBER_PERCENT", percent: 10 },
    ]);
    render(
      <ResultSummary
        quote={quote}
        promotions={promotions}
        onRecalculate={vi.fn()}
      />,
    );

    const items = screen.getByRole("region", { name: "Discounted items" });
    expect(within(items).getByText(/1 pair · 5% off/)).toBeInTheDocument();
    // 240.00 - 12.00 = 228.00 is what the member percent applies to.
    expect(screen.getByText(/10% of .*228\.00/)).toBeInTheDocument();
  });

  it("lists every product that was ordered with quantity, unit price and line total", () => {
    const multi: Quote = {
      ...quote,
      items: [
        {
          productId: "orange",
          name: "Orange set",
          quantity: 2,
          unitPriceSatang: 12000,
          lineTotalSatang: 24000,
        },
        {
          productId: "blue",
          name: "Blue set",
          quantity: 3,
          unitPriceSatang: 3000,
          lineTotalSatang: 9000,
        },
      ],
      totalBeforeSatang: 33000,
    };
    render(<ResultSummary quote={multi} onRecalculate={vi.fn()} />);

    const order = screen.getByRole("region", { name: "Your order" });
    const rows = within(order).getAllByRole("listitem");
    expect(rows).toHaveLength(2);
    expect(rows[0]).toHaveTextContent("Orange set");
    expect(rows[0]).toHaveTextContent(/2\s*×\s*฿120\.00/);
    expect(rows[0]).toHaveTextContent(/฿240\.00/);
    expect(rows[1]).toHaveTextContent("Blue set");
    expect(rows[1]).toHaveTextContent(/3\s*×\s*฿30\.00/);
    expect(rows[1]).toHaveTextContent(/฿90\.00/);
  });
});
