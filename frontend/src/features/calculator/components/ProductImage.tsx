"use client";

import { useEffect, useRef, useState } from "react";

/** Served by Next.js itself, so it works even when the API cannot be reached. */
const FALLBACK_SRC = "/no-image.svg";

type Props = { src: string; className?: string };

/**
 * A product picture that can never look broken. The API already answers with a
 * default picture when it has no image, so this second fallback only covers the
 * browser failing to reach the API at all.
 */
export function ProductImage({ src, className }: Props) {
  const [failed, setFailed] = useState(false);
  const ref = useRef<HTMLImageElement>(null);

  // The image can fail before React attaches onError (server-rendered markup
  // that loaded before hydration), so check once on mount as well.
  useEffect(() => {
    const img = ref.current;
    if (
      img &&
      img.complete &&
      img.naturalWidth === 0 &&
      img.currentSrc !== ""
    ) {
      setFailed(true);
    }
  }, []);

  return (
    // A plain <img>: the picture comes from our own API (not a static asset
    // next/image can optimise) and has its own fallback.
    // eslint-disable-next-line @next/next/no-img-element
    <img
      ref={ref}
      src={failed ? FALLBACK_SRC : src}
      alt=""
      width={72}
      height={72}
      loading="lazy"
      decoding="async"
      onError={() => setFailed(true)}
      className={className}
    />
  );
}
