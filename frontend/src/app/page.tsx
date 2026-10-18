import { Suspense } from "react";
import { connection } from "next/server";
import { Calculator } from "@/features/calculator/components/Calculator";
import { TEST_TOOLS_ENABLED } from "@/lib/env";
import { getProducts, getPromotions } from "@/features/calculator/server";

export default function Page() {
  return (
    <main className="mx-auto flex min-h-dvh w-full max-w-md flex-col px-4 pt-10">
      <h1 className="mb-2 text-3xl font-semibold tracking-tight">Food Store</h1>
      <Suspense fallback={<MenuSkeleton />}>
        <Menu />
      </Suspense>
    </main>
  );
}

// The API is not reachable while the app is built, so the menu is rendered at
// request time. The static shell above is served immediately.
async function Menu() {
  await connection();
  const [products, promotions] = await Promise.all([
    getProducts(),
    getPromotions(),
  ]);
  return (
    <Calculator
      products={products}
      promotions={promotions}
      showTestTools={TEST_TOOLS_ENABLED}
    />
  );
}

function MenuSkeleton() {
  return (
    <div
      className="flex flex-col gap-3"
      aria-busy="true"
      aria-label="Loading menu"
    >
      {Array.from({ length: 7 }, (_, i) => (
        <div
          key={i}
          className="flex items-center gap-4 rounded-3xl bg-white p-3 pr-4"
        >
          <div className="size-[72px] rounded-2xl bg-field motion-safe:animate-pulse" />
          <div className="flex-1">
            <div className="h-4 w-28 rounded bg-field motion-safe:animate-pulse" />
            <div className="mt-2 h-3 w-16 rounded bg-field motion-safe:animate-pulse" />
          </div>
          <div className="size-11 rounded-full bg-field motion-safe:animate-pulse" />
        </div>
      ))}
    </div>
  );
}
