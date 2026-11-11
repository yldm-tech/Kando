// Open source: the dark panel with the repository and the downloads for each system, a tab each.
import site from "virtual:kando/site";
import { useEffect } from "react";
import { useWords } from "../../i18n/content";
import { debPackage, installCommand, macDisk, readmeURL, releaseFileURL, releasesURL, repoURL, windowsSetup } from "../../lib/site";
import { CodeBlock } from "../common/CodeBlock";
import { AppleMark, GitHubMark } from "../common/marks";
import { SlidingPill } from "../common/SlidingPill";
import { useTabs } from "../common/tabs";

// systems are the download panel's tabs, by the name a link to the panel gives in its data-os.
const systems = ["mac", "linux", "win"] as const;

export function OpenSource() {
  const { t, lang } = useWords();
  // Each tab's buttons download its system's packages of this release, one per architecture.
  const file = (name: (version: string, arch: string) => string, arch: string) => releaseFileURL(site.version, name(site.version, arch));
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
                <p className="flex flex-wrap items-center gap-2.5">
                  <a className="btn primary" href={file(macDisk, "arm64")}>
                    <AppleMark size={16} />
                    {t("dlMacButton")}
                  </a>
                  <a className="btn soft" href={file(macDisk, "amd64")}>
                    {t("dlMacIntel")}
                  </a>
                </p>
                <p className="fine">{t("dlFirstRun")}</p>
              </div>
              <div className="dl-panel" {...tabs.panel(1)}>
                <p>{t("dlLinuxBody")}</p>
                <CodeBlock text={installCommand} />
                <p className="fine">{t("dlLinuxNote")}</p>
                <p className="flex flex-wrap items-center gap-2.5">
                  <a className="btn soft" href={file(debPackage, "amd64")}>
                    {t("dlLinuxButton")}
                  </a>
                  <a className="btn soft" href={file(debPackage, "arm64")}>
                    {t("dlLinuxArm")}
                  </a>
                </p>
                <p className="fine">{t("dlFirstRun")}</p>
              </div>
              <div className="dl-panel" {...tabs.panel(2)}>
                <p>{t("dlWinBody")}</p>
                <p className="flex flex-wrap items-center gap-2.5">
                  <a className="btn primary" href={file(windowsSetup, "amd64")}>
                    {t("dlWinButton")}
                  </a>
                  <a className="btn soft" href={file(windowsSetup, "arm64")}>
                    {t("dlWinArm")}
                  </a>
                </p>
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
