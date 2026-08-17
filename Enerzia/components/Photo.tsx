'use client';

import { useEffect, useRef, useState } from 'react';

import type { Photo as PhotoData } from '@/lib/content/types';

/**
 * A photograph in a caller-supplied frame, with the frame's gradient showing
 * through if the file is missing or fails to load.
 *
 * Shared by `/farm` (nine photographs) and the homepage hero. It exists as one
 * component rather than two because of the second note below, which is the kind
 * of bug that gets re-introduced the moment the logic is copied.
 *
 * 1. **The failure is tracked in React state, not by touching the DOM.** An
 *    `onError` that sets `style.display = 'none'` is never reset by React, so
 *    one failed load leaves the element hidden forever — the exact defect found
 *    on the PDP in task 9.5. Here the error flips a state flag and the gradient
 *    renders instead, which a re-render can undo.
 *
 * 2. **`onError` alone is not enough on a server-rendered image.** The `<img>`
 *    ships inside the SSR HTML, so the browser starts fetching it long before
 *    React hydrates. A 404 that lands in that gap fires an `error` event with
 *    no handler attached yet, and the fallback never runs — the frame is left
 *    showing sprawled alt text over the gradient, which is what `/farm` did on
 *    first load until the mount check below was added. So we also ask the
 *    element on mount whether it has *already* failed.
 *
 * 3. **Plain `<img>`, not `next/image`.** The files are owner-supplied and a
 *    slot can legitimately be empty; `next/image` treats a missing local file
 *    as an error, which would take the whole page down rather than degrading
 *    one frame.
 *
 * If you replace an image file, **change its filename**. The browser caches by
 * URL, and a same-name replacement keeps serving the old picture — recorded in
 * handoff.md because it already cost a full round of debugging once.
 *
 * `frameClassName` owns the frame entirely — its size, radius and shadow — so
 * the homepage hero keeps `.home-hero-art` and `/farm` keeps its own shapes,
 * with no class fighting between them. The frame must set `overflow: hidden`.
 */
export function Photo({
  photo,
  frameClassName,
  /** Eager-load and skip lazy loading. Use only for above-the-fold images. */
  priority = false,
}: {
  photo: PhotoData;
  frameClassName: string;
  priority?: boolean;
}) {
  const [failed, setFailed] = useState(false);
  const imgRef = useRef<HTMLImageElement>(null);

  // Catches a load that failed before hydration attached `onError` — note 2
  // above. `complete` with a zero `naturalWidth` is the standard test for
  // "this image is done, and there is nothing in it".
  useEffect(() => {
    const el = imgRef.current;
    if (el?.complete && el.naturalWidth === 0) setFailed(true);
  }, []);

  return (
    <div className={frameClassName} style={{ background: photo.gradient }}>
      {!failed && (
        // eslint-disable-next-line @next/next/no-img-element
        <img
          ref={imgRef}
          className="photo-img"
          src={photo.src}
          alt={photo.alt}
          loading={priority ? 'eager' : 'lazy'}
          decoding="async"
          onError={() => setFailed(true)}
        />
      )}
    </div>
  );
}
