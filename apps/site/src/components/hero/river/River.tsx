// The hero's backdrop: a warm river of light, Three.js particles drifting across behind the headline. It is a chunk of its own, loaded once the page has painted and the browser is idle, and only where the reader has not asked for less motion, the connection neither saves data nor is slow, and the browser draws WebGL, which is tested only then; otherwise, and until it fades in, the stylesheet's glows are the whole backdrop.
import { lazy, Suspense, useEffect, useState } from "react";
import { lessMotion } from "../../common/motion";

const RiverScene = lazy(() => import("./RiverScene"));

// canDraw is whether the browser gives a canvas a WebGL context. The context it makes to find out is released at once, as browsers keep only a few alive.
function canDraw(): boolean {
  const canvas = document.createElement("canvas");
  const gl = canvas.getContext("webgl2") ?? canvas.getContext("webgl");
  if (!gl) {
    return false;
  }
  gl.getExtension("WEBGL_lose_context")?.loseContext();
  return true;
}

// sparing is whether the reader's connection asks the page to save data, or is slow: the river's code is the largest the page loads, for a backdrop the glows stand in for.
function sparing(): boolean {
  const connection = (navigator as Navigator & { connection?: { saveData?: boolean; effectiveType?: string } }).connection;
  return connection?.saveData === true || ["slow-2g", "2g", "3g"].includes(connection?.effectiveType ?? "");
}

export function River() {
  const [wanted, setWanted] = useState(false);
  const [gone, setGone] = useState(false);

  useEffect(() => {
    if (lessMotion() || sparing()) {
      return;
    }
    const idles = "requestIdleCallback" in window;
    let idle = 0;
    let timer = 0;
    const later = () => {
      // The river waits for the entrance to have started, so that the first frames go to the page itself.
      timer = window.setTimeout(() => {
        if (idles) {
          idle = window.requestIdleCallback(() => setWanted(canDraw()), { timeout: 1500 });
        } else {
          setWanted(canDraw());
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
