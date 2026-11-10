// SlidingPill is the marker behind the chosen item of a row of tabs or links, which slides to the next one when the choice changes, on the design's curve (.4s, cubic-bezier(.4, 0, .2, 1)). It is drawn only once the page runs: before that, and without script, the stylesheet marks the chosen item itself, and the row's data-pill attribute, set with the pill, tells the stylesheet to stop. The row must be positioned, so that the items' offsets are the pill's.
import { animate } from "motion";
import { useLayoutEffect, useRef } from "react";
import { lessMotion, slide } from "./motion";

export function SlidingPill({ index, items, className }: { index: number; items: string; className: string }) {
  const pill = useRef<HTMLSpanElement>(null);
  const placed = useRef(false);

  useLayoutEffect(() => {
    const el = pill.current;
    const row = el?.parentElement;
    if (!el || !row) {
      return;
    }
    const place = (moving: boolean) => {
      const item = index >= 0 ? row.querySelectorAll<HTMLElement>(items)[index] : undefined;
      if (!item) {
        void animate(el, { opacity: 0 }, { duration: moving && placed.current ? 0.2 : 0 });
        return;
      }
      const to = { x: item.offsetLeft, y: item.offsetTop, width: item.offsetWidth, height: item.offsetHeight, opacity: 1 };
      if (!moving || !placed.current || lessMotion()) {
        void animate(el, to, { duration: 0 });
      } else {
        void animate(el, to, { duration: 0.4, ease: slide });
      }
      placed.current = true;
    };
    row.setAttribute("data-pill", "");
    place(true);
    // The row's items change size as fonts load or the window narrows; the pill follows without sliding.
    const resized = new ResizeObserver(() => place(false));
    resized.observe(row);
    return () => resized.disconnect();
  }, [index, items]);

  return <span ref={pill} className={"pointer-events-none absolute top-0 left-0 z-0 opacity-0 " + className} aria-hidden="true" />;
}
