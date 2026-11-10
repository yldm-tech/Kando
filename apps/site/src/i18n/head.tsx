// The head of a page: its title and description, canonical address, the same page in the other languages, the link preview tags, and the scripts every page runs.
import site from "virtual:kando/site";
import { languages, pageURL, siteURL, sourceLang, type LangCode } from "../lib/site";
import { jsonForScript } from "../lib/json";

// PageKind is the kind of page: a language's home page, its catalog of websites, or 404.html, which serves every missing address.
export type PageKind = "home" | "sites" | "404";

// PageHead is what the root document needs to know of the page it renders.
export interface PageHead {
  kind: PageKind;
  code: LangCode;
  title: string;
  description: string;
  imageAlt: string;
}

const langs = languages.filter((l) => l.code in site.shots);

// HeadTags are the page's own tags, after the ones every page has.
export function HeadTags({ page }: { page: PageHead }) {
  const lang = langs.find((l) => l.code === page.code) ?? languages[0];
  if (!lang) {
    throw new Error("no languages");
  }
  if (page.kind === "404") {
    return (
      <>
        <title>{page.title}</title>
        <meta name="robots" content="noindex" />
        <script src={site.assets.siteJS} />
      </>
    );
  }
  const group = page.kind;
  const url = pageURL(page.code, group);
  const shot = site.shots[page.code]?.library;
  if (!shot) {
    throw new Error(`no library screenshot for ${page.code}`);
  }
  const image = siteURL + shot.src;
  return (
    <>
      <title>{page.title}</title>
      <meta name="description" content={page.description} />
      <link rel="canonical" href={url} />
      {langs.map((l) => (
        <link key={l.code} rel="alternate" hrefLang={l.tag} href={pageURL(l.code, group)} data-lang={l.code} />
      ))}
      <link rel="alternate" hrefLang="x-default" href={pageURL(sourceLang, group)} />
      <meta property="og:type" content="website" />
      <meta property="og:site_name" content="Kando" />
      <meta property="og:title" content={page.title} />
      <meta property="og:description" content={page.description} />
      <meta property="og:url" content={url} />
      <meta property="og:locale" content={lang.locale} />
      {langs
        .filter((l) => l.code !== page.code)
        .map((l) => (
          <meta key={l.code} property="og:locale:alternate" content={l.locale} />
        ))}
      <meta property="og:image" content={image} />
      <meta property="og:image:width" content={String(shot.w)} />
      <meta property="og:image:height" content={String(shot.h)} />
      <meta property="og:image:alt" content={page.imageAlt} />
      <meta name="twitter:card" content="summary_large_image" />
      <meta name="twitter:title" content={page.title} />
      <meta name="twitter:description" content={page.description} />
      <meta name="twitter:image" content={image} />
      <meta name="twitter:image:alt" content={page.imageAlt} />
      <script src={site.assets.siteJS} />
      <script src={site.assets.webmcpJS} defer />
      <script type="application/json" id="webmcp-data" dangerouslySetInnerHTML={{ __html: jsonForScript(site.webmcp[page.code]) }} />
    </>
  );
}
