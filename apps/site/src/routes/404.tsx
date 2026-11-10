import { createFileRoute } from "@tanstack/react-router";
import { NotFoundPage } from "../components/not-found/NotFoundPage";
import { loadPage } from "../i18n/page";

// 404.html: GitHub Pages serves it at any missing address, so it is rendered once, in every language there is.
export const Route = createFileRoute("/404")({
  loader: () => loadPage("en", "404"),
  component: NotFoundPage,
});
