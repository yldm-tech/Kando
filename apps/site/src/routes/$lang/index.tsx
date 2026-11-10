import { createFileRoute } from "@tanstack/react-router";
import { HomePage } from "../../components/home/HomePage";
import { loadPage, otherLang } from "../../i18n/page";

export const Route = createFileRoute("/$lang/")({
  loader: ({ params }) => loadPage(otherLang(params.lang), "home"),
  component: HomePage,
});
