import { createRouter } from "@tanstack/react-router";
import { routeTree } from "./routeTree.gen";

// getRouter is the site's router. Every page is prerendered and the language links load a page anew, so the router only renders the page it was loaded on; addresses end with a slash, as the folders GitHub Pages serves them from.
export function getRouter() {
  return createRouter({ routeTree, trailingSlash: "always", scrollRestoration: false, defaultPreload: false });
}

declare module "@tanstack/react-router" {
  interface Register {
    router: ReturnType<typeof getRouter>;
  }
}
