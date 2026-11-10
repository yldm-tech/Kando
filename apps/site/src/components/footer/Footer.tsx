// The footer, one row: the brand and the license, the languages, and the project's links.
import { useWords } from "../../i18n/content";
import { issuesURL, licenseURL, pagePath, releasesURL, repoURL } from "../../lib/site";
import { Brand } from "../common/Brand";
import { LangLinks } from "../header/LangLinks";

export function Footer({ page }: { page: "home" | "sites" | "404" }) {
  const { t, lang } = useWords();
  return (
    <footer className="foot">
      <div className="wrap foot-in">
        <div className="foot-brand">
          <a className="brand small" href={pagePath(lang.code)} aria-label={t("homeLabel")}>
            <Brand />
          </a>
          <a className="foot-license" href={licenseURL}>
            {t("footerLicense")}
          </a>
        </div>
        <nav className="foot-langs" aria-label={t("languageLabel")}>
          <LangLinks page={page} />
        </nav>
        <nav className="foot-links" aria-label={t("footerLinksLabel")}>
          <a href={repoURL}>{t("footerGitHub")}</a>
          <a href={releasesURL}>{t("footerReleases")}</a>
          <a href={issuesURL}>{t("footerIssues")}</a>
        </nav>
      </div>
    </footer>
  );
}
