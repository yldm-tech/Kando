import { createFileRoute } from "@tanstack/react-router";
import { PluginPage } from "../../../components/plugin-page/PluginPage";
import { loadPage, otherLang } from "../../../i18n/page";

export const Route = createFileRoute("/$lang/plugin/")({
  loader: ({ params }) => loadPage(otherLang(params.lang), "plugin"),
  component: PluginPage,
});
