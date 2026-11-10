import { createFileRoute } from "@tanstack/react-router";
import { SitesPage } from "../../../components/sites-page/SitesPage";
import { loadPage, otherLang } from "../../../i18n/page";

export const Route = createFileRoute("/$lang/sites/")({
  loader: ({ params }) => loadPage(otherLang(params.lang), "sites"),
  component: SitesPage,
});
