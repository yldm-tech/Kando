// A language's page of Kando's community plugins. The directory is not part of the build: the page is rendered with the list loading, and once it runs it fetches the signed index Kando downloads (PluginDirectory), so it lists what Kando would offer at that moment.
import { useWords } from "../../i18n/content";
import { pluginsGuideURL, pluginsRepoURL } from "../../lib/site";
import { Footer } from "../footer/Footer";
import { Header } from "../header/Header";
import { PluginDirectory } from "./PluginDirectory";

export function PluginPage() {
  const { t } = useWords();
  return (
    <>
      <Header page="plugin" />
      <main id="main" className="relative pt-[132px] pb-[100px] phone:pt-[108px]">
        <div className="pointer-events-none absolute inset-x-0 top-0 h-[520px] overflow-hidden" aria-hidden="true">
          <span className="blob blob-a" />
          <span className="blob blob-b" />
        </div>
        <div className="wrap relative">
          <header className="head mx-auto max-w-[720px]">
            <h1 id="plugin-title" className="text-[clamp(34px,4.6vw,56px)] leading-[1.12] font-extrabold tracking-[-0.03em]">
              {t("pluginPageTitle")}
            </h1>
            <p className="mx-auto mt-[18px] max-w-[600px] text-[17px] leading-[1.75] text-ink-2">{t("pluginPageLead")}</p>
          </header>
          <div className="mx-auto mt-10 max-w-[1040px]">
            <PluginDirectory />
          </div>
          <section className="plugin-contribute mx-auto mt-20 max-w-[720px] text-center" aria-labelledby="plugin-contribute-title">
            <h2 id="plugin-contribute-title" className="text-[clamp(24px,3vw,32px)] leading-[1.2] font-extrabold tracking-[-0.02em]">
              {t("pluginContributeTitle")}
            </h2>
            <p className="mx-auto mt-3 max-w-[560px] text-[16px] leading-[1.7] text-ink-2">{t("pluginContributeBody")}</p>
            <div className="mt-6 flex flex-wrap items-center justify-center gap-3">
              <a className="btn primary" href={pluginsGuideURL}>
                {t("pluginContributeGuide")}
              </a>
              <a className="btn ghost" href={pluginsRepoURL}>
                {t("pluginContributeRepo")}
              </a>
            </div>
          </section>
        </div>
      </main>
      <Footer page="plugin" />
    </>
  );
}
