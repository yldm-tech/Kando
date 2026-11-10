// The header: the brand, the section links in a pill that marks the section being read, the language menu and the GitHub link.
import { SlidingPill } from "../common/SlidingPill";
import { useEffect, useRef, useState } from "react";
import { useWords, type ContentKey } from "../../i18n/content";
import { pagePath, repoURL } from "../../lib/site";
import { Brand } from "../common/Brand";
import { GitHubMark } from "../common/marks";
import { LangLinks } from "./LangLinks";

// sections are the header's links in the page's order, as the design has them: Features leads to the four steps at the top of what Kando does, so the pill moves left to right as the page scrolls down.
const sections: readonly [string, ContentKey][] = [
  ["how", "navFeatures"],
  ["cli", "navCli"],
  ["open", "navOpenSource"],
  ["faq", "navFaq"],
];

// Header is the page's header; on a page that is not a language's home page (sites, 404) the section links lead to the home page's sections, or are left out where the page has no language (404).
export function Header({ page }: { page: "home" | "sites" | "404" }) {
  const { t, lang } = useWords();
  const top = useRef<HTMLElement>(null);
  const [scrolled, setScrolled] = useState(false);
  const [current, setCurrent] = useState("");
  const base = page === "home" ? "" : pagePath(lang.code);

  // The header turns solid once the page scrolls, and on the home page its pill marks the section being read.
  useEffect(() => {
    let pending = false;
    const update = () => {
      pending = false;
      setScrolled(window.scrollY > 12);
      if (page !== "home") {
        return;
      }
      // The section being read is the lowest one whose top has passed 40% of the window: by where the sections are on the page, not the order of the links, so a section placed after another is never shadowed by it.
      let now = "";
      let lowest = -Infinity;
      for (const [id] of sections) {
        const top = document.getElementById(id)?.getBoundingClientRect().top;
        if (top !== undefined && top < window.innerHeight * 0.4 && top > lowest) {
          lowest = top;
          now = id;
        }
      }
      setCurrent(now);
    };
    const onScroll = () => {
      if (!pending) {
        pending = true;
        window.requestAnimationFrame(update);
      }
    };
    window.addEventListener("scroll", onScroll, { passive: true });
    update();
    return () => window.removeEventListener("scroll", onScroll);
  }, [page]);

  return (
    <>
      <a className="skip" href="#main">
        {t("skipToContent")}
      </a>
      <header className={"top" + (scrolled ? " scrolled" : "")} ref={top}>
        <div className="wrap top-in">
          <a className="brand" href={pagePath(lang.code)} aria-label={t("homeLabel")}>
            <Brand first />
          </a>
          {page !== "404" && (
            <nav className="nav" aria-label={t("navLabel")}>
              <SlidingPill
                index={sections.findIndex(([id]) => id === current)}
                items="a"
                className="rounded-[16px] bg-white shadow-[0_1px_2px_rgba(28,25,22,0.08),0_4px_12px_rgba(28,25,22,0.08)]"
              />
              {sections.map(([id, key]) => (
                <a key={id} href={base + "#" + id} className={current === id ? "on" : undefined}>
                  {t(key)}
                </a>
              ))}
            </nav>
          )}
          <div className="top-end">
            <details className="lang">
              <summary aria-label={t("languageLabel") + ": " + lang.name}>
                <svg className="i" viewBox="0 0 24 24" width="16" height="16" aria-hidden="true">
                  <circle cx="12" cy="12" r="9" />
                  <path d="M3 12h18M12 3c2.6 2.8 2.6 15.2 0 18M12 3c-2.6 2.8-2.6 15.2 0 18" />
                </svg>
                <span className="lang-name">{lang.name}</span>
                <svg className="i chev" viewBox="0 0 24 24" width="12" height="12" aria-hidden="true">
                  <path d="m6 9 6 6 6-6" />
                </svg>
              </summary>
              <ul className="lang-menu">
                <LangLinks page={page} item />
              </ul>
            </details>
            <a className="gh" href={repoURL} aria-label={t("githubLabel")}>
              <GitHubMark size={16} />
              <span>GitHub</span>
            </a>
          </div>
        </div>
      </header>
    </>
  );
}
