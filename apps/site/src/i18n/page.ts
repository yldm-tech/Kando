import { notFound } from "@tanstack/react-router";
import type { PageData } from "../routes/__root";
import { otherLangCodes, type LangCode, type OtherLangCode } from "../lib/site";
import { loadContent, words } from "./content";
import type { PageKind } from "./head";

// loadPage is the data of a language's page of the kind, for its route's loader.
export async function loadPage(code: LangCode, kind: PageKind): Promise<PageData> {
  const content = await loadContent(code);
  const { t } = words(content);
  switch (kind) {
    case "home":
      return { head: { kind, code, title: t("metaTitle"), description: t("metaDescription"), imageAlt: t("metaImageAlt") }, content };
    case "sites":
      return { head: { kind, code, title: t("sitesMetaTitle"), description: t("sitesMetaDescription"), imageAlt: t("metaImageAlt") }, content };
    case "404":
      return { head: { kind, code, title: t("notFoundTitle") + " · Kando", description: "", imageAlt: "" }, content };
  }
}

// otherLang is the $lang segment of a page's address as a language code, or a not-found for a folder that is no language's.
export function otherLang(segment: string): OtherLangCode {
  const code = otherLangCodes.find((c) => c === segment);
  if (!code) {
    throw notFound();
  }
  return code;
}
