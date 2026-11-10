// llms.txt and llms-full.txt, after the llms.txt proposal (llmstxt.org): what Kando is, in Markdown, for a language model that reads the site, with links to its pages in every language, the READMEs and the newest release. llms.txt is the short index; llms-full.txt has the page's text itself, in English: how it works, the features, how to install it and the questions. Both are made from the content files and the same agent content as the WebMCP tools (buildtime/webmcp.ts), so that they say what the pages say.
import { writeFileSync } from "node:fs";
import { join } from "node:path";
import {
  faqPairs,
  issuesURL,
  latestURL,
  licenseURL,
  pageURL,
  readmeURL,
  releasesURL,
  repoURL,
  siteURL,
  sourceLang,
  type Language,
  type LangCode,
} from "../src/lib/site";
import { fill, type Strings } from "./content";
import { plainLine, type AgentContent } from "./webmcp";

export interface LLMsInput {
  langs: readonly Language[];
  content: ReadonlyMap<LangCode, Strings>;
  version: string;
  // total is the number of websites in the catalog.
  total: number;
  // agent is the English agent content: the Get started steps and the features.
  agent: AgentContent;
}

// steps are the How it works section's steps, by the prefix of their Title and Body keys.
const steps = ["stepDownload", "stepTranscribe", "stepTranslate", "stepExport"] as const;

function words(strs: Strings, total: number, version: string): (key: string) => string {
  return (key) => {
    const s = strs[key];
    if (s === undefined) {
      throw new Error(`no content key ${key}`);
    }
    return plainLine(fill(key, s, { count: total, version }).replaceAll("`", ""));
  };
}

function head(t: (key: string) => string): string {
  return `# Kando\n\n> ${t("metaDescription")}\n\n${t("heroLead")}\n\n${t("heroMeta")}\n`;
}

// llmsTxt is llms.txt: the summary, then the pages, the documentation and the rest as lists of links.
export function llmsTxt(p: LLMsInput): string {
  const en = p.content.get(sourceLang);
  if (!en) {
    throw new Error("no English content");
  }
  const t = words(en, p.total, p.version);
  // tl is a language's words, for the links to its pages and README, which are described in that language.
  const tl = (l: Language) => words(p.content.get(l.code) ?? {}, p.total, p.version);
  const pages = [
    ...p.langs.map((l) => `- [${l.name}](${pageURL(l.code)}): ${tl(l)("metaTitle")}`),
    ...p.langs.map((l) => `- [${l.name}: ${tl(l)("sitesPageTitle")}](${pageURL(l.code, "sites")}): ${tl(l)("sitesMetaDescription")}`),
  ];
  const docs = [
    ...p.langs.map((l) => `- [README (${l.name})](${readmeURL(l)}): ${tl(l)("dlReadme")}`),
    `- [Kando ${p.version}](${releasesURL}/tag/v${p.version}): ${t("dlMacBody")} ${t("dlFirstRun")}`,
    `- [llms-full.txt](${siteURL}/llms-full.txt): The English page in full: how it works, the features, how to install it and the questions.`,
  ];
  const optional = [`- [GitHub](${repoURL}): ${t("openBody")}`, `- [${t("footerIssues")}](${issuesURL})`, `- [${t("footerLicense")}](${licenseURL})`];
  return [head(t), "## Pages\n\n" + pages.join("\n") + "\n", "## Docs\n\n" + docs.join("\n") + "\n", "## Optional\n\n" + optional.join("\n") + "\n"].join("\n");
}

// llmsFullTxt is llms-full.txt: the English page's text as Markdown.
export function llmsFullTxt(p: LLMsInput): string {
  const en = p.content.get(sourceLang);
  if (!en) {
    throw new Error("no English content");
  }
  const t = words(en, p.total, p.version);
  const out: string[] = [head(t)];
  out.push(`## ${t("stepsTitle")}\n\n` + steps.map((s, i) => `${i + 1}. **${t(s + "Title")}**: ${t(s + "Body")}`).join("\n") + "\n");
  const features = p.agent.features.items.map((f) => `### ${f.title}\n\n${f.text}` + (f.link ? `\n\n[${f.link.text}](${f.link.url})` : "") + "\n");
  out.push(`## ${p.agent.features.title}\n\n` + features.join("\n"));
  const start = p.agent.start.steps.map((s) => {
    let md = `### ${s.title}\n\n${s.text.join(" ")}\n`;
    if (s.command) {
      md += "\n```sh\n" + s.command + "\n```\n";
    }
    if (s.notes && s.notes.length > 0) {
      md += "\n" + s.notes.join(" ") + "\n";
    }
    if (s.link) {
      md += `\n[${s.link.text}](${s.link.url})\n`;
    }
    return md;
  });
  out.push(`## ${p.agent.start.title}\n\n${p.agent.start.lead}\n\n` + start.join("\n") + `\n[${p.agent.start.readme.text}](${p.agent.start.readme.url})\n`);
  out.push(`## ${t("faqTitle")}\n\n` + faqPairs.map(([q, a]) => `### ${t(q)}\n\n${t(a)}\n`).join("\n"));
  out.push(
    `## Links\n\n- [${t("sitesPageTitle")}](${pageURL(sourceLang, "sites")})\n- [GitHub](${repoURL})\n- [${t("footerReleases")}](${latestURL})\n- [${t("footerLicense")}](${licenseURL})\n`,
  );
  return out.join("\n");
}

export function writeLLMsFiles(out: string, p: LLMsInput): void {
  writeFileSync(join(out, "llms.txt"), llmsTxt(p));
  writeFileSync(join(out, "llms-full.txt"), llmsFullTxt(p));
}
