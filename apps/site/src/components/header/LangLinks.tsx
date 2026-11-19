// LangLinks are the links to the page in each language, as the header's menu and the footer list them, the page's own marked as the current page. 404.html is no language's page, so it marks none and links the home pages.
import site from "virtual:kando/site";
import { useWords } from "../../i18n/content";
import type { PageKind } from "../../i18n/head";
import { languages, pagePath } from "../../lib/site";

export function LangLinks({ page, item = false }: { page: PageKind; item?: boolean }) {
  const { lang } = useWords();
  const langs = languages.filter((l) => l.code in site.shots);
  return (
    <>
      {langs.map((l) => {
        const link = (
          <a
            key={l.code}
            href={pagePath(l.code, page === "404" ? "home" : page)}
            hrefLang={l.tag}
            lang={l.tag}
            data-lang={l.code}
            aria-current={page !== "404" && l.code === lang.code ? "page" : undefined}
          >
            {l.name}
          </a>
        );
        return item ? <li key={l.code}>{link}</li> : link;
      })}
    </>
  );
}
