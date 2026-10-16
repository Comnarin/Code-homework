import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import ErrorPage from "./error";

describe("error page", () => {
  it("Try again calls retry so the menu is re-fetched, not just re-rendered from the stale failure", async () => {
    const retry = vi.fn();
    render(<ErrorPage error={new Error("boom")} retry={retry} />);

    await userEvent.click(screen.getByRole("button", { name: "Try again" }));

    expect(retry).toHaveBeenCalledTimes(1);
  });
});
