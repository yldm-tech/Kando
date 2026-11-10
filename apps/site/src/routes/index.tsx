import { createFileRoute } from "@tanstack/react-router";
import { HomePage } from "../components/home/HomePage";
import { loadPage } from "../i18n/page";

export const Route = createFileRoute("/")({
  loader: () => loadPage("en", "home"),
  component: HomePage,
});
