// Open source: the dark panel with the repository and the downloads for each system, a tab each.
import { useEffect } from "react";
import { useWords } from "../../i18n/content";
import { buildCommands, installCommand, latestURL, readmeURL, releasesURL, repoURL } from "../../lib/site";
import { CodeBlock } from "../common/CodeBlock";
import { AppleMark, GitHubMark } from "../common/marks";
import { SlidingPill } from "../common/SlidingPill";
import { useTabs } from "../common/tabs";

// systems are the download panel's tabs, by the name a link to the panel gives in its data-os.
const systems = ["mac", "linux", "win"] as const;

export function OpenSource() {
  const { t, lang } = useWords();
  const tabs = useTabs([
    ["tab-mac", "panel-mac"],
    ["tab-linux", "panel-linux"],
    ["tab-win", "panel-win"],
  ]);
  const { select } = tabs;
  // A link to the download panel that names a system, as Windows and Linux under the headline, opens that system's tab.
  useEffect(() => {
    const onClick = (event: MouseEvent) => {
      const link = event.target instanceof Element ? event.target.closest("a[data-os]") : null;
      const i = systems.indexOf((link?.getAttribute("data-os") ?? "") as (typeof systems)[number]);
      if (i >= 0) {
        select(i);
      }
    };
    document.addEventListener("click", onClick);
    return () => document.removeEventListener("click", onClick);
  }, [select]);
  return (
    <section className="section" id="open" aria-labelledby="open-title">
      <div className="wrap">
        <div className="dark-panel rv">
          <div className="glow glow-c" aria-hidden="true" />
          <div className="open-text">
            <p className="eyebrow">{t("openEyebrow")}</p>
            <h2 id="open-title">{t("openTitle")}</h2>
            <p className="body">{t("openBody")}</p>
            <div className="actions">
              <a className="btn white" href={repoURL}>
                <GitHubMark size={16} />
                {t("openGitHub")}
              </a>
              <a className="btn outline" href={releasesURL}>
                {t("openReleases")}
              </a>
            </div>
          </div>
          <div className="dl" id="start">
            <div className="seg" role="tablist" aria-label={t("dlTabsLabel")}>
              <SlidingPill index={tabs.selected} items='[role="tab"]' className="rounded-[9px] bg-paper shadow-[0_2px_8px_rgba(0,0,0,0.3)]" />
              <button {...tabs.tab(0)} data-os="mac">
                macOS
              </button>
              <button {...tabs.tab(1)} data-os="linux">
                Linux
              </button>
              <button {...tabs.tab(2)} data-os="win">
                Windows
              </button>
            </div>
            <div {...tabs.stack}>
              <div className="dl-panel" {...tabs.panel(0)}>
                <p>{t("dlMacBody")}</p>
                <p>
                  <a className="btn primary" href={latestURL}>
                    <AppleMark size={16} />
                    {t("dlMacButton")}
                  </a>
                </p>
                <p className="fine">{t("dlFirstRun")}</p>
              </div>
              <div className="dl-panel" {...tabs.panel(1)}>
                <p>{t("dlLinuxBody")}</p>
                <CodeBlock text={installCommand} />
                <p className="fine">{t("dlLinuxNote")}</p>
                <p>
                  <a className="btn soft" href={latestURL}>
                    {t("dlLinuxButton")}
                  </a>
                </p>
                <p className="fine">{t("dlFirstRun")}</p>
              </div>
              <div className="dl-panel" {...tabs.panel(2)}>
                <p>{t("dlWinBody")}</p>
                <CodeBlock text={buildCommands} />
                <p className="fine">{t("dlWinNote")}</p>
              </div>
            </div>
            <p className="dl-readme">
              <a href={readmeURL(lang)}>
                {t("dlReadme")} <span aria-hidden="true">→</span>
              </a>
            </p>
          </div>
        </div>
      </div>
    </section>
  );
}
