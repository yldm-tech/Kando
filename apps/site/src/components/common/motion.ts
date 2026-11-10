// Motion on the page: the hero's entrance, the sections' reveals as they scroll in, a tab panel's arrival and a question opening and closing. All of it runs after hydration on elements the server rendered complete, so the page reads the same without script; where the reader asked for less motion none of it runs.
import { animate, inView, stagger } from "motion";
import { useEffect, type RefObject } from "react";

// ease is the design's curve, the stylesheet's --ease.
export const ease = [0.2, 0.7, 0.2, 1] as const;

// lessMotion is whether the reader asked for less motion.
export function lessMotion(): boolean {
  return window.matchMedia("(prefers-reduced-motion: reduce)").matches;
}

// useEntrance plays the hero's entrance on the elements under root marked data-enter, one after another in the page's order; the one marked data-enter="stage", the player's window, rises last and from further down. The stylesheet hides them from the moment the head script marks the page as running script, so that they do not show before the entrance starts, and shows them after a few seconds if the page's script never runs.
export function useEntrance(root: RefObject<HTMLElement | null>) {
  useEffect(() => {
    const all = [...(root.current?.querySelectorAll<HTMLElement>("[data-enter]") ?? [])];
    const stage = all.filter((el) => el.dataset["enter"] === "stage");
    const text = all.filter((el) => el.dataset["enter"] !== "stage");
    if (lessMotion()) {
      for (const el of all) {
        el.style.opacity = "1";
      }
      return;
    }
    const controls = [
      animate(text, { opacity: [0, 1], y: [26, 0], filter: ["blur(8px)", "blur(0px)"] }, { duration: 1, ease, delay: stagger(0.12, { startDelay: 0.05 }) }),
      animate(stage, { opacity: [0, 1], y: [70, 0], scale: [0.97, 1] }, { duration: 1.4, ease, delay: 0.55 }),
    ];
    return () => {
      for (const c of controls) {
        c.complete();
      }
    };
  }, [root]);
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

// arrive brings in a tab panel that has just been shown.
export function arrive(el: HTMLElement | null) {
  if (!el || lessMotion()) {
    return;
  }
  void animate(el, { opacity: [0, 1], y: [10, 0], filter: ["blur(6px)", "blur(0px)"] }, { duration: 0.4, ease }).then(() => {
    el.style.removeProperty("filter");
    el.style.removeProperty("transform");
  });
}

// unfold opens a disclosure's answer from nothing to its height; fold closes it, and then calls done, which closes the disclosure itself. The answer's bottom padding goes with its height, so that it folds to nothing.
export function unfold(el: HTMLElement | null) {
  if (!el || lessMotion()) {
    return;
  }
  const height = el.getBoundingClientRect().height;
  const pad = getComputedStyle(el).paddingBottom;
  el.style.overflow = "hidden";
  void animate(el, { height: [0, height], paddingBottom: ["0px", pad], opacity: [0, 1] }, { duration: 0.38, ease }).then(() => settle(el));
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

// settle gives an answer's size back to the stylesheet.
function settle(el: HTMLElement) {
  for (const name of ["height", "padding-bottom", "opacity", "overflow"]) {
    el.style.removeProperty(name);
  }
}
