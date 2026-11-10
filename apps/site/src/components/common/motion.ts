// Motion on the page: the sections' reveals as they scroll in, a question opening and closing, the player's side panel sliding open and shut and its subtitle's lines changing places. A tab panel's arrival is the stylesheet's (k-panel, components/common/tabs.ts). All of it runs after hydration on elements the server rendered complete, so the page reads the same without script; where the reader asked for less motion none of it runs. The hero's entrance is the stylesheet's, so that the first screen never waits for hydration.
import { animate, inView } from "motion";
import { useEffect, type RefObject } from "react";

// ease is the design's curve, the stylesheet's --ease.
export const ease = [0.2, 0.7, 0.2, 1] as const;

// slide is the design's curve for what moves from one place to another, as a tab's pill does: the stylesheet's --ease-slide.
export const slide = [0.4, 0, 0.2, 1] as const;

// lessMotion is whether the reader asked for less motion.
export function lessMotion(): boolean {
  return window.matchMedia("(prefers-reduced-motion: reduce)").matches;
}

// useReveals lifts every element marked .rv under root into place as it scrolls into view, once. Elements already on screen when the page hydrates stay as they are, so that nothing the reader sees blinks; those below are hidden first, where the reader cannot see it happen. Siblings that come into view together follow one another by a beat.
export function useReveals(root: RefObject<HTMLElement | null>) {
  useEffect(() => {
    const el = root.current;
    if (!el || lessMotion()) {
      return;
    }
    const below = [...el.querySelectorAll<HTMLElement>(".rv")].filter((rv) => rv.getBoundingClientRect().top > window.innerHeight);
    for (const rv of below) {
      rv.style.opacity = "0";
      rv.style.transform = "translateY(56px)";
    }
    const stops = below.map((rv) =>
      inView(
        rv,
        () => {
          const siblings = rv.parentElement ? [...rv.parentElement.children].filter((c) => c.classList.contains("rv")) : [rv];
          const beat = (siblings.indexOf(rv) % 3) * 0.09;
          // The element gives its transform back to the stylesheet once in place, for the hover lifts the stylesheet gives some of them.
          void animate(rv, { opacity: [0, 1], transform: ["translateY(56px)", "translateY(0px)"] }, { duration: 0.9, ease, delay: beat }).then(() => {
            rv.style.removeProperty("transform");
            rv.style.removeProperty("opacity");
          });
        },
        { amount: 0.12 },
      ),
    );
    return () => {
      for (const stop of stops) {
        stop();
      }
      for (const rv of below) {
        rv.style.removeProperty("transform");
        rv.style.removeProperty("opacity");
      }
    };
  }, [root]);
}

// unfold opens a disclosure's answer from nothing to its height, rising into place as the design's k-open does; fold closes it, and then calls done, which closes the disclosure itself. The answer's bottom padding goes with its height, so that it folds to nothing.
export function unfold(el: HTMLElement | null) {
  if (!el || lessMotion()) {
    return;
  }
  const height = el.getBoundingClientRect().height;
  const pad = getComputedStyle(el).paddingBottom;
  el.style.overflow = "hidden";
  void animate(el, { height: [0, height], paddingBottom: ["0px", pad], opacity: [0, 1], y: [-6, 0] }, { duration: 0.38, ease }).then(() => settle(el));
}

export function fold(el: HTMLElement | null, done: () => void) {
  if (!el || lessMotion()) {
    done();
    return;
  }
  const height = el.getBoundingClientRect().height;
  const pad = getComputedStyle(el).paddingBottom;
  el.style.overflow = "hidden";
  void animate(el, { height: [height, 0], paddingBottom: [pad, "0px"], opacity: [1, 0] }, { duration: 0.28, ease }).then(() => {
    done();
    settle(el);
  });
}

// settle gives an element's size back to the stylesheet.
function settle(el: HTMLElement) {
  for (const name of ["height", "padding-top", "padding-right", "padding-bottom", "padding-left", "opacity", "overflow", "transform", "flex-basis"]) {
    el.style.removeProperty(name);
  }
}

// hold keeps el where it is on the screen for ms while what is above it grows or shrinks, by scrolling the page as much as el moved each frame, so that what the reader clicked stays under the pointer.
export function hold(el: HTMLElement | null, ms: number) {
  if (!el || lessMotion()) {
    return;
  }
  const top = el.getBoundingClientRect().top;
  const start = performance.now();
  const step = () => {
    const moved = el.getBoundingClientRect().top - top;
    if (moved !== 0) {
      window.scrollBy({ top: moved, behavior: "instant" });
    }
    if (performance.now() - start < ms) {
      window.requestAnimationFrame(step);
    }
  };
  window.requestAnimationFrame(step);
}

// drawer opens or closes the player's side panel. Beside the picture it slides across, its width going to nothing or coming from it while the picture keeps its height (the stylesheet sizes the picture from the player's width); under the picture, on a narrow screen, it folds up or down. Closing leaves the panel at nothing for done to hide it; opening starts from there, on a panel that has just been shown.
export function drawer(el: HTMLElement | null, open: boolean, done: () => void = () => {}) {
  if (!el || lessMotion()) {
    if (el) {
      settle(el);
    }
    done();
    return;
  }
  const main = el.previousElementSibling;
  const across = main instanceof HTMLElement && main.offsetTop === el.offsetTop;
  if (open) {
    settle(el);
  }
  // The panel's padding along the way goes with its size, so that it slides to nothing and nothing is left to jump when it is hidden.
  const style = getComputedStyle(el);
  const sides = across ? (["paddingLeft", "paddingRight"] as const) : (["paddingTop", "paddingBottom"] as const);
  const from = (full: string) => (open ? ["0px", full] : [full, "0px"]);
  const size = across ? el.getBoundingClientRect().width : el.getBoundingClientRect().height;
  el.style.overflow = "hidden";
  const to: Record<string, string[] | number[]> = { [across ? "flexBasis" : "height"]: from(size + "px"), opacity: open ? [0, 1] : [1, 0] };
  for (const side of sides) {
    to[side] = from(style[side]);
  }
  void animate(el, to, { duration: 0.35, ease: slide }).then(() => {
    if (open) {
      settle(el);
    }
    done();
  });
}

// flip moves els from where they were, at the tops given, to where they are now, as when the subtitle's two lines swap places.
export function flip(els: HTMLElement[], tops: number[]) {
  if (lessMotion()) {
    return;
  }
  els.forEach((el, i) => {
    const moved = (tops[i] ?? 0) - el.getBoundingClientRect().top;
    if (moved !== 0) {
      void animate(el, { y: [moved, 0] }, { duration: 0.4, ease: slide }).then(() => el.style.removeProperty("transform"));
    }
  });
}
