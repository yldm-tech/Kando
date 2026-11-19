import { createFileRoute } from "@tanstack/react-router";
import { PluginPage } from "../../components/plugin-page/PluginPage";
import { loadPage } from "../../i18n/page";

export const Route = createFileRoute("/plugin/")({
  loader: () => loadPage("en", "plugin"),
  component: PluginPage,
});
