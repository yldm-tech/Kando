import { createFileRoute } from "@tanstack/react-router";
import { PrivacyPage } from "../../../components/privacy-page/PrivacyPage";
import { loadPage, otherLang } from "../../../i18n/page";

export const Route = createFileRoute("/$lang/privacy/")({
  loader: ({ params }) => loadPage(otherLang(params.lang), "privacy"),
  component: PrivacyPage,
});
