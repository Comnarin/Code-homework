import type { CalculateRequest, Quote } from "./api";

/** Must match the API's MaxQuantityPerLine. */
export const MAX_QUANTITY = 1000;

export type State = {
  quantities: Record<string, number>;
  memberCard: string;
  status: "idle" | "loading" | "result" | "error";
  quote: Quote | null;
  errorMessage: string | null;
};

export type Action =
  | { type: "increment"; productId: string }
  | { type: "decrement"; productId: string }
  | { type: "setMemberCard"; value: string }
  | { type: "submit" }
  | { type: "succeeded"; quote: Quote }
  | { type: "failed"; message: string }
  | { type: "recalculate" };

export const initialState: State = {
  quantities: {},
  memberCard: "",
  status: "idle",
  quote: null,
  errorMessage: null,
};

export function calculatorReducer(state: State, action: Action): State {
  switch (action.type) {
    case "increment": {
      const next = Math.min(
        (state.quantities[action.productId] ?? 0) + 1,
        MAX_QUANTITY,
      );
      return {
        ...state,
        quantities: { ...state.quantities, [action.productId]: next },
      };
    }
    case "decrement": {
      const next = Math.max((state.quantities[action.productId] ?? 0) - 1, 0);
      return {
        ...state,
        quantities: { ...state.quantities, [action.productId]: next },
      };
    }
    case "setMemberCard":
      return { ...state, memberCard: action.value };
    case "submit":
      return { ...state, status: "loading", errorMessage: null };
    case "succeeded":
      return {
        ...state,
        status: "result",
        quote: action.quote,
        errorMessage: null,
      };
    case "failed":
      return { ...state, status: "error", errorMessage: action.message };
    case "recalculate":
      return { ...state, status: "idle", quote: null, errorMessage: null };
  }
}

export function buildRequest(state: State): CalculateRequest {
  const items = Object.entries(state.quantities)
    .filter(([, quantity]) => quantity > 0)
    .map(([productId, quantity]) => ({ productId, quantity }));
  const memberCard = state.memberCard.trim();
  return memberCard ? { memberCard, items } : { items };
}

export function hasItems(state: State): boolean {
  return Object.values(state.quantities).some((q) => q > 0);
}
