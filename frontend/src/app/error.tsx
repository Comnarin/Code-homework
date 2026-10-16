"use client";

export default function Error({ reset }: { error: Error; reset: () => void }) {
  return (
    <main className="mx-auto flex min-h-screen max-w-md flex-col items-center justify-center gap-4 bg-slate-50 p-4 text-center text-slate-900">
      <h1 className="text-xl font-bold">We could not load the menu</h1>
      <p className="text-slate-600">The store service is not responding. Please try again.</p>
      <button type="button" onClick={reset} className="rounded-full bg-sky-500 px-6 py-2 text-white hover:bg-sky-600">
        Try again
      </button>
    </main>
  );
}
