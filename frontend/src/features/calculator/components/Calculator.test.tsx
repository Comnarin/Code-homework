import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { CalculateResult, Promotion } from "../api";
import { Calculator } from "./Calculator";

const products = [
  { id: "red", name: "Red set", priceSatang: 5000 },
  { id: "pink", name: "Pink set", priceSatang: 8000 },
];

const okResult: CalculateResult = {
  ok: true,
  quote: {
    items: [
      {
        productId: "pink",
        name: "Pink set",
        quantity: 2,
        unitPriceSatang: 8000,
        lineTotalSatang: 16000,
      },
    ],
    totalBeforeSatang: 16000,
    itemDiscounts: [],
    memberDiscountSatang: 1600,
    finalTotalSatang: 14400,
  },
};

describe("Calculator", () => {
  it("disables Calculate until something is in the cart", async () => {
    render(<Calculator products={products} calculate={vi.fn()} />);
    const button = screen.getByRole("button", { name: "Calculate" });
    expect(button).toBeDisabled();

    await userEvent.click(
      screen.getByRole("button", { name: "Increase Pink set" }),
    );

    expect(button).toBeEnabled();
  });

  it("sends only chosen items and the trimmed card, then shows the result", async () => {
    const calculate = vi.fn().mockResolvedValue(okResult);
    render(<Calculator products={products} calculate={calculate} />);

    await userEvent.click(
      screen.getByRole("button", { name: "Increase Pink set" }),
    );
    await userEvent.click(
      screen.getByRole("button", { name: "Increase Pink set" }),
    );
    await userEvent.type(screen.getByLabelText("Member card"), "  MEM0001 ");
    await userEvent.click(screen.getByRole("button", { name: "Calculate" }));

    expect(calculate).toHaveBeenCalledWith({
      memberCard: "MEM0001",
      items: [{ productId: "pink", quantity: 2 }],
    });
    expect(await screen.findByText("Final total")).toBeInTheDocument();
    expect(screen.getByText("Final total").parentElement).toHaveTextContent(
      /144\.00/,
    );
  });

  it("Recalculate goes back to the form with the cart intact", async () => {
    render(
      <Calculator
        products={products}
        calculate={vi.fn().mockResolvedValue(okResult)}
      />,
    );
    await userEvent.click(
      screen.getByRole("button", { name: "Increase Pink set" }),
    );
    await userEvent.click(screen.getByRole("button", { name: "Calculate" }));

    await userEvent.click(
      await screen.findByRole("button", { name: "Recalculate" }),
    );

    expect(screen.getByLabelText("Pink set quantity")).toHaveTextContent("1");
  });

  it("shows the server message and keeps the cart when the Red set is held", async () => {
    const calculate = vi.fn().mockResolvedValue({
      ok: false,
      code: "LIMITED_PRODUCT_UNAVAILABLE",
      message:
        "Red set is reserved by another customer right now. Please try again later.",
    });
    render(<Calculator products={products} calculate={calculate} />);
    await userEvent.click(
      screen.getByRole("button", { name: "Increase Red set" }),
    );

    await userEvent.click(screen.getByRole("button", { name: "Calculate" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Red set is reserved by another customer",
    );
    expect(screen.getByLabelText("Red set quantity")).toHaveTextContent("1");
  });

  it("shows a retryable message when the network fails and lets the user retry", async () => {
    const calculate = vi
      .fn()
      .mockResolvedValueOnce({
        ok: false,
        code: "NETWORK",
        message: "Cannot reach the server. Please try again.",
      })
      .mockResolvedValueOnce(okResult);
    render(<Calculator products={products} calculate={calculate} />);
    await userEvent.click(
      screen.getByRole("button", { name: "Increase Pink set" }),
    );

    await userEvent.click(screen.getByRole("button", { name: "Calculate" }));
    expect(await screen.findByRole("alert")).toHaveTextContent(/try again/i);

    await userEvent.click(screen.getByRole("button", { name: "Calculate" }));
    expect(await screen.findByText("Final total")).toBeInTheDocument();
  });

  it("prevents double submission while a request is in flight", async () => {
    let finish: (r: CalculateResult) => void = () => {};
    const calculate = vi
      .fn()
      .mockReturnValue(new Promise<CalculateResult>((r) => (finish = r)));
    render(<Calculator products={products} calculate={calculate} />);
    await userEvent.click(
      screen.getByRole("button", { name: "Increase Pink set" }),
    );

    await userEvent.click(screen.getByRole("button", { name: "Calculate" }));

    expect(screen.getByRole("button", { name: /calculating/i })).toBeDisabled();
    expect(calculate).toHaveBeenCalledTimes(1);
    finish(okResult);
    expect(await screen.findByText("Final total")).toBeInTheDocument();
  });

  it("locks the cart and card while a request is in flight so the result matches what was sent", async () => {
    let finish: (r: CalculateResult) => void = () => {};
    const calculate = vi
      .fn()
      .mockReturnValue(new Promise<CalculateResult>((r) => (finish = r)));
    render(<Calculator products={products} calculate={calculate} />);
    await userEvent.click(
      screen.getByRole("button", { name: "Increase Pink set" }),
    );
    await userEvent.click(screen.getByRole("button", { name: "Calculate" }));

    expect(
      screen.getByRole("button", { name: "Increase Pink set" }),
    ).toBeDisabled();
    expect(
      screen.getByRole("button", { name: "Decrease Pink set" }),
    ).toBeDisabled();
    expect(screen.getByLabelText("Member card")).toBeDisabled();

    finish(okResult);
    expect(await screen.findByText("Final total")).toBeInTheDocument();
  });

  it("shows promotion badges and the member percent from the live rules, not from code", () => {
    const promotions: Promotion[] = [
      {
        code: "PAIR",
        type: "BUNDLE_PERCENT",
        percentBp: 700,
        bundleSize: 2,
        productIds: ["red"],
      },
      { code: "MEMBER", type: "MEMBER_PERCENT", percentBp: 1500 },
    ];
    render(
      <Calculator
        products={products}
        promotions={promotions}
        calculate={vi.fn()}
      />,
    );

    expect(screen.getByText(/7% off each pair/)).toBeInTheDocument();
    expect(screen.getByText(/Members get 15% off/)).toBeInTheDocument();
    // Pink has no rule in this data, so it gets no badge.
    expect(screen.getAllByText(/off each pair/)).toHaveLength(1);
  });

  it("hides the test tools unless they are enabled", () => {
    render(<Calculator products={products} calculate={vi.fn()} />);
    expect(
      screen.queryByRole("button", { name: /reset red set limit/i }),
    ).not.toBeInTheDocument();
  });

  it("resets the Red set limit from the test tools and confirms it", async () => {
    const reset = vi.fn().mockResolvedValue({ ok: true });
    render(
      <Calculator
        products={products}
        calculate={vi.fn()}
        showTestTools
        reset={reset}
      />,
    );

    await userEvent.click(
      screen.getByRole("button", { name: /reset red set limit/i }),
    );

    expect(reset).toHaveBeenCalledTimes(1);
    expect(await screen.findByRole("status")).toHaveTextContent(
      /limit cleared/i,
    );
  });

  it("reports a failed reset", async () => {
    const reset = vi
      .fn()
      .mockResolvedValue({ ok: false, message: "Could not reset the limit." });
    render(
      <Calculator
        products={products}
        calculate={vi.fn()}
        showTestTools
        reset={reset}
      />,
    );

    await userEvent.click(
      screen.getByRole("button", { name: /reset red set limit/i }),
    );

    expect(await screen.findByRole("status")).toHaveTextContent(
      "Could not reset the limit.",
    );
  });
});
