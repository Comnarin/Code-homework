import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ProductRow } from "./ProductRow";

const pink = {
  id: "pink",
  name: "Pink set",
  priceSatang: 8000,
  imageUrl: "/api/products/pink/image",
};

describe("ProductRow", () => {
  it("shows the name, price and quantity", () => {
    render(
      <ProductRow
        product={pink}
        quantity={3}
        onIncrement={vi.fn()}
        onDecrement={vi.fn()}
      />,
    );
    expect(screen.getByText("Pink set")).toBeInTheDocument();
    expect(screen.getByText(/80\.00/)).toBeInTheDocument();
    expect(screen.getByLabelText("Pink set quantity")).toHaveTextContent("3");
  });

  it("calls the handlers from the labelled buttons", async () => {
    const onIncrement = vi.fn();
    const onDecrement = vi.fn();
    render(
      <ProductRow
        product={pink}
        quantity={1}
        onIncrement={onIncrement}
        onDecrement={onDecrement}
      />,
    );

    await userEvent.click(
      screen.getByRole("button", { name: "Increase Pink set" }),
    );
    await userEvent.click(
      screen.getByRole("button", { name: "Decrease Pink set" }),
    );

    expect(onIncrement).toHaveBeenCalledTimes(1);
    expect(onDecrement).toHaveBeenCalledTimes(1);
  });

  it("shows the promotion label when the product has one", () => {
    render(
      <ProductRow
        product={pink}
        quantity={0}
        promoLabel="5% off each pair"
        onIncrement={vi.fn()}
        onDecrement={vi.fn()}
      />,
    );
    expect(screen.getByText(/5% off each pair/)).toBeInTheDocument();
  });

  it("offers only add at zero", () => {
    render(
      <ProductRow
        product={pink}
        quantity={0}
        onIncrement={vi.fn()}
        onDecrement={vi.fn()}
      />,
    );
    expect(
      screen.queryByRole("button", { name: "Decrease Pink set" }),
    ).not.toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Increase Pink set" }),
    ).toBeInTheDocument();
  });

  it("shows the product picture from its imageUrl", () => {
    const { container } = render(
      <ProductRow
        product={pink}
        quantity={0}
        onIncrement={vi.fn()}
        onDecrement={vi.fn()}
      />,
    );
    const img = container.querySelector("img");
    expect(img).not.toBeNull();
    expect(img).toHaveAttribute("src", "/api/products/pink/image");
    // The name is right beside it, so the picture is decorative for screen readers.
    expect(img).toHaveAttribute("alt", "");
  });

  it("falls back to the default picture when the image cannot load", () => {
    const { container } = render(
      <ProductRow
        product={pink}
        quantity={0}
        onIncrement={vi.fn()}
        onDecrement={vi.fn()}
      />,
    );
    const img = container.querySelector("img")!;

    fireEvent.error(img);

    expect(img).toHaveAttribute("src", "/no-image.svg");
  });
});
