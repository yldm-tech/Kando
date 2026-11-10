// The hero's backdrop: a warm river of light, Three.js particles drifting across behind the headline. It is a chunk of its own, loaded once the page has painted and the browser is idle, and only where the reader has not asked for less motion and the browser draws WebGL; otherwise, and until it fades in, the stylesheet's glows are the whole backdrop.
import { lazy, Suspense, useEffect, useState } from "react";
import { lessMotion } from "../../common/motion";

const RiverScene = lazy(() => import("./RiverScene"));

// canDraw is whether the browser gives a canvas a WebGL context.
function canDraw(): boolean {
  const canvas = document.createElement("canvas");
  return (canvas.getContext("webgl2") ?? canvas.getContext("webgl")) !== null;
}

export function River() {
  const [wanted, setWanted] = useState(false);
  const [gone, setGone] = useState(false);

  useEffect(() => {
    if (lessMotion() || !canDraw()) {
      return;
    }
    const idles = "requestIdleCallback" in window;
    let idle = 0;
    let timer = 0;
    const later = () => {
      // The river waits for the entrance to have started, so that the first frames go to the page itself.
      timer = window.setTimeout(() => {
        if (idles) {
          idle = window.requestIdleCallback(() => setWanted(true), { timeout: 1500 });
        } else {
          setWanted(true);
        }
      }, 450);
    };
    if (document.readyState === "complete") {
      later();
    } else {
      window.addEventListener("load", later, { once: true });
    }
    return () => {
      window.removeEventListener("load", later);
      window.clearTimeout(timer);
      if (idles) {
        window.cancelIdleCallback(idle);
      }
    };
  }, []);

  if (!wanted || gone) {
    return null;
  }
  return (
    <Suspense fallback={null}>
      <RiverScene onGiveUp={() => setGone(true)} />
    </Suspense>
  );
}
