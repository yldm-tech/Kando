import { createFileRoute } from "@tanstack/react-router";
import { SitesPage } from "../../components/sites-page/SitesPage";
import { loadPage } from "../../i18n/page";

export const Route = createFileRoute("/sites/")({
  loader: () => loadPage("en", "sites"),
  component: SitesPage,
});
