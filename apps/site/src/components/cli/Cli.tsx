// Command line & MCP: what the kando command and its MCP server offer, counted from the app's source, beside a terminal with an example of each.
import { Fragment, type ReactNode } from "react";
import site from "virtual:kando/site";
import { Rich, useWords } from "../../i18n/content";
import { SlidingPill } from "../common/SlidingPill";
import { useTabs } from "../common/tabs";

// Lines are the lines of a <pre>, joined by line breaks, as the terminal shows them.
function Lines({ lines }: { lines: ReactNode[] }) {
  return (
    <>
      {lines.map((line, i) => (
        <Fragment key={i}>
          {i > 0 && "\n"}
          {line}
        </Fragment>
      ))}
    </>
  );
}

export function Cli() {
  const { t, tf } = useWords();
  const tabs = useTabs([
    ["tab-cli", "panel-cli"],
    ["tab-mcp", "panel-mcp"],
  ]);
  const prompt = <span className="p">$ </span>;
  return (
    <section className="section split" id="cli" aria-labelledby="cli-title">
      <div className="wrap split-in">
        <div className="split-text rv">
          <p className="eyebrow">{t("cliEyebrow")}</p>
          <h2 id="cli-title">
            <Rich text={t("cliTitle")} />
          </h2>
          <p className="body">
            <Rich text={t("cliBody")} />
          </p>
          <ul className="points">
            <li>
              <Rich text={tf("cliPoint1", { commands: site.surface.commands })} />
            </li>
            <li>
              <Rich text={tf("cliPoint2", { tools: site.surface.tools.length })} />
            </li>
            <li>{t("cliPoint3")}</li>
          </ul>
        </div>
        <div className="term rv">
          <div className="term-bar">
            <span className="lights" aria-hidden="true">
              <i />
              <i />
              <i />
            </span>
            <div className="seg" role="tablist" aria-label={t("cliTabsLabel")}>
              <SlidingPill index={tabs.selected} items='[role="tab"]' className="rounded-[9px] bg-paper shadow-[0_2px_8px_rgba(0,0,0,0.3)]" />
              <button {...tabs.tab(0)}>{t("cliTabCommands")}</button>
              <button {...tabs.tab(1)}>MCP</button>
            </div>
          </div>
          <div className="term-body" {...tabs.panel(0)}>
            <pre className="typed">
              <code>
                <Lines
                  lines={[
                    <span key="c1" className="ln c">
                      {"# " + t("cliCommentAdd")}
                    </span>,
                    <span key="l1" className="ln">
                      {prompt}
                      <span className="k">kando</span> <span className="s">add</span> <span className="u">https://www.youtube.com/watch?v=…</span>{" "}
                      <span className="f">--wait</span>
                    </span>,
                    <span key="c2" className="ln c">
                      {"# " + t("cliCommentList")}
                    </span>,
                    <span key="l2" className="ln">
                      {prompt}
                      <span className="k">kando</span> <span className="s">list</span> <span className="f">--source</span> youtube
                    </span>,
                    <span key="c3" className="ln c">
                      {"# " + t("cliCommentSubtitles")}
                    </span>,
                    <span key="l3" className="ln">
                      {prompt}
                      <span className="k">kando</span> <span className="s">subtitles</span> <span className="h">{"<film-id>"}</span>{" "}
                      <span className="f">--format</span> text <span className="f">--translation</span>
                    </span>,
                    <span key="c4" className="ln c">
                      {"# " + t("cliCommentStatus")}
                    </span>,
                    <span key="l4" className="ln">
                      {prompt}
                      <span className="k">kando</span> <span className="s">status</span> <span className="f">--json</span>
                    </span>,
                  ]}
                />
              </code>
            </pre>
          </div>
          <div className="term-body mcp" {...tabs.panel(1)}>
            <p className="term-label">
              <img src={site.claudeCode.src} width="16" height="16" alt="" />
              {t("mcpCommentAdd")}
            </p>
            <pre>
              <code>
                {prompt}
                <span className="k">claude</span> <span className="s">mcp</span> add kando <span className="o">--</span>{" "}
                <span className="u">/Applications/Kando.app/Contents/MacOS/Kando</span> mcp
              </code>
            </pre>
            <p className="term-label">{t("mcpCommentTools")}</p>
            <ul className="tool-names">
              {site.surface.tools.map((name) => (
                <li key={name}>
                  <code>{name}</code>
                </li>
              ))}
            </ul>
            <p className="term-note">{t("mcpOther")}</p>
          </div>
        </div>
      </div>
    </section>
  );
}
