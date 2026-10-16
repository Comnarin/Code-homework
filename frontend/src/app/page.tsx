import { Suspense } from "react";
import { connection } from "next/server";
import { Calculator } from "@/features/calculator/components/Calculator";
import { getProducts } from "@/features/calculator/server";

export default function Page() {
  return (
    <main className="mx-auto flex min-h-screen w-full max-w-md flex-col gap-6 bg-slate-50 p-4 text-slate-900">
      <h1 className="text-xl font-bold">Food Store Calculator</h1>
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
  const products = await getProducts();
  return <Calculator products={products} />;
}

function MenuSkeleton() {
  return (
    <div className="flex flex-col gap-3" aria-busy="true" aria-label="Loading menu">
      {Array.from({ length: 7 }, (_, i) => (
        <div key={i} className="h-[72px] animate-pulse rounded-2xl bg-slate-200" />
      ))}
    </div>
  );
}
