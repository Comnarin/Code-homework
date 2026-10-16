"use client";

// `retry` re-fetches the failed segment and then resets the boundary. `reset`
// alone would re-render the same, already-failed payload.
export default function ErrorPage({
  retry,
}: {
  error: unknown;
  retry: () => void;
}) {
  return (
    <main className="mx-auto flex min-h-dvh max-w-md flex-col items-center justify-center gap-4 p-4 text-center">
      <h1 className="text-2xl font-semibold tracking-tight">
        We could not load the menu
      </h1>
      <p className="text-ink-soft">
        The store service is not responding. Please try again.
      </p>
      <button
        type="button"
        onClick={retry}
        className="h-12 rounded-full bg-ink px-8 font-medium text-white transition-colors duration-150 hover:bg-ink/90 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent"
      >
        Try again
      </button>
    </main>
  );
}
