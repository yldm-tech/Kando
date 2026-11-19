import { createFileRoute } from "@tanstack/react-router";
import { PrivacyPage } from "../../components/privacy-page/PrivacyPage";
import { loadPage } from "../../i18n/page";

export const Route = createFileRoute("/privacy/")({
  loader: () => loadPage("en", "privacy"),
  component: PrivacyPage,
});
