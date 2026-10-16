"use client";

import { useCallback, useReducer } from "react";
import type { CalculateFn } from "./api";
import { buildRequest, calculatorReducer, initialState } from "./state";

export function useCalculator(calculate: CalculateFn) {
  const [state, dispatch] = useReducer(calculatorReducer, initialState);

  const submit = useCallback(async () => {
    dispatch({ type: "submit" });
    const result = await calculate(buildRequest(state));
    dispatch(result.ok ? { type: "succeeded", quote: result.quote } : { type: "failed", message: result.message });
  }, [calculate, state]);

  return {
    state,
    increment: (productId: string) => dispatch({ type: "increment", productId }),
    decrement: (productId: string) => dispatch({ type: "decrement", productId }),
    setMemberCard: (value: string) => dispatch({ type: "setMemberCard", value }),
    submit,
    recalculate: () => dispatch({ type: "recalculate" }),
  };
}
