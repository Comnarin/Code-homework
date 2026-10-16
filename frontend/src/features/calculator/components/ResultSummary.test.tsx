import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { Quote } from "../api";
import { ResultSummary } from "./ResultSummary";

const quote: Quote = {
  totalBeforeSatang: 24000,
  itemDiscounts: [{ ruleCode: "PAIR_5", productId: "orange", name: "Orange set", bundles: 1, amountSatang: 1200 }],
  memberDiscountSatang: 2280,
  finalTotalSatang: 20520,
};

describe("ResultSummary", () => {
  it("shows total, item discounts, member discount and final total", () => {
    render(<ResultSummary quote={quote} onRecalculate={vi.fn()} />);

    expect(screen.getByText("Total (before discount)").parentElement).toHaveTextContent(/240\.00/);
    const items = screen.getByRole("region", { name: "Discounted items" });
    expect(within(items).getByText("Orange set")).toBeInTheDocument();
    expect(within(items).getByText(/-.*12\.00/)).toBeInTheDocument();
    expect(screen.getByText("Discount from member card").parentElement).toHaveTextContent(/-.*22\.80/);
    expect(screen.getByText("Final total").parentElement).toHaveTextContent(/205\.20/);
  });

  it("says so when no item got a discount", () => {
    render(<ResultSummary quote={{ ...quote, itemDiscounts: [], memberDiscountSatang: 0 }} onRecalculate={vi.fn()} />);
    expect(screen.getByText(/no item discounts/i)).toBeInTheDocument();
  });

  it("recalculates on click", async () => {
    const onRecalculate = vi.fn();
    render(<ResultSummary quote={quote} onRecalculate={onRecalculate} />);
    await userEvent.click(screen.getByRole("button", { name: "Recalculate" }));
    expect(onRecalculate).toHaveBeenCalledTimes(1);
  });
});
