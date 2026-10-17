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
    <div aria-busy="true" aria-label="Loading menu">
      {Array.from({ length: 7 }, (_, i) => (
        <div
          key={i}
          className="flex h-[72px] items-center gap-3 border-b border-line last:border-b-0"
        >
          <div className="size-4 rounded-full bg-line motion-safe:animate-pulse" />
          <div className="h-4 w-28 rounded bg-line motion-safe:animate-pulse" />
        </div>
      ))}
    </div>
  );
}
