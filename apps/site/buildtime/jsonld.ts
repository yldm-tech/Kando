// The structured data of each page, as JSON-LD in a <script type="application/ld+json"> data block, which the browser never runs, so the Content Security Policy stays as strict as it is (buildtime/csp.ts leaves data blocks out of the hashes). Every page describes the app (SoftwareApplication), its publisher (Organization) and the site in its language (WebSite); a home page adds its questions (FAQPage) and a sites page the way back to its home page (BreadcrumbList). Each graph is checked with the zod schemas below, so that a missing or misshapen field fails the build rather than reaching the search engines. The data is put into the prerendered HTML after the build (finishHTML), so that it adds nothing to the scripts the pages load.
import { z } from "zod";
import { faqPairs, languages, latestURL, licenseURL, pageURL, repoURL, siteURL, type LangCode } from "../src/lib/site";
import { fill, type Strings } from "./content";
import type { Shot } from "./schema";
import { plainLine } from "./webmcp";

export const schemaContext = "https://schema.org";

// The entities every page names, by their @id: one app and one publisher, whatever the page's language.
export const appID = siteURL + "/#app";
export const orgID = siteURL + "/#organization";

const text = z.string().refine((s) => s.trim() !== "" && !s.includes("`") && !/\{[a-zA-Z]+\}/.test(s), "is empty or has markup or a placeholder left in it");
const url = z.url({ protocol: /^https$/ });
const ref = z.strictObject({ "@id": url });

export const organizationSchema = z.strictObject({
  "@type": z.literal("Organization"),
  "@id": z.literal(orgID),
  name: z.literal("YLDM"),
  sameAs: z.array(url).min(1),
});

export const imageObjectSchema = z.strictObject({
  "@type": z.literal("ImageObject"),
  url,
  width: z.number().int().positive(),
  height: z.number().int().positive(),
  caption: text,
});

export const softwareApplicationSchema = z.strictObject({
  "@type": z.literal("SoftwareApplication"),
  "@id": z.literal(appID),
  name: z.literal("Kando"),
  // alternateName is the app's name in the page's language where it has one of its own, such as 看懂.
  alternateName: text.optional(),
  description: text,
  url,
  image: url,
  operatingSystem: z.literal("macOS, Linux, Windows"),
  applicationCategory: z.literal("MultimediaApplication"),
  offers: z.strictObject({ "@type": z.literal("Offer"), price: z.literal("0"), priceCurrency: z.literal("USD") }),
  isAccessibleForFree: z.literal(true),
  softwareVersion: z.string().regex(/^\d+\.\d+\.\d+$/),
  downloadUrl: z.literal(latestURL),
  license: z.literal(licenseURL),
  screenshot: imageObjectSchema,
  // inLanguage are the app's interface languages, the site's languages.
  inLanguage: z.array(z.string().min(2)).min(1),
  author: ref,
  publisher: ref,
});

export const webSiteSchema = z.strictObject({
  "@type": z.literal("WebSite"),
  "@id": url,
  url,
  name: z.literal("Kando"),
  alternateName: text.optional(),
  description: text,
  inLanguage: z.string().min(2),
  publisher: ref,
  about: ref,
});

export const faqPageSchema = z.strictObject({
  "@type": z.literal("FAQPage"),
  "@id": url,
  url,
  inLanguage: z.string().min(2),
  mainEntity: z
    .array(
      z.strictObject({
        "@type": z.literal("Question"),
        name: text,
        acceptedAnswer: z.strictObject({ "@type": z.literal("Answer"), text }),
      }),
    )
    .min(1),
});

export const breadcrumbListSchema = z.strictObject({
  "@type": z.literal("BreadcrumbList"),
  "@id": url,
  itemListElement: z
    .array(z.strictObject({ "@type": z.literal("ListItem"), position: z.number().int().positive(), name: text, item: url }))
    .min(2)
    .refine((items) => items.every((it, i) => it.position === i + 1), "positions are not 1, 2, 3…"),
});

export const graphSchema = z.strictObject({
  "@context": z.literal(schemaContext),
  "@graph": z.array(z.discriminatedUnion("@type", [organizationSchema, softwareApplicationSchema, webSiteSchema, faqPageSchema, breadcrumbListSchema])).min(3),
});
export type Graph = z.infer<typeof graphSchema>;

export interface PageInput {
  code: LangCode;
  kind: "home" | "sites";
  strings: Strings;
  version: string;
  // total is the number of websites in the catalog, which an answer names.
  total: number;
  shot: Shot;
}

// structuredData is the graph of a page, checked against graphSchema.
export function structuredData(p: PageInput): Graph {
  const lang = languages.find((l) => l.code === p.code);
  if (!lang) {
    throw new Error("no language " + p.code);
  }
  const t = (key: string): string => {
    const s = p.strings[key];
    if (s === undefined) {
      throw new Error(`no content key ${key} in ${p.code}`);
    }
    // The page sets the names between ` in code and breaks some headings in two; structured data is plain text.
    return plainLine(fill(key, s, { count: p.total }).replaceAll("`", ""));
  };
  const brand = t("brandName");
  const alternateName = brand === "Kando" ? undefined : brand;
  const home = pageURL(p.code);
  const graph: unknown[] = [
    { "@type": "Organization", "@id": orgID, name: "YLDM", sameAs: [repoURL] },
    {
      "@type": "SoftwareApplication",
      "@id": appID,
      name: "Kando",
      alternateName,
      description: t("metaDescription"),
      url: home,
      image: siteURL + "/icon-512.png",
      operatingSystem: "macOS, Linux, Windows",
      applicationCategory: "MultimediaApplication",
      offers: { "@type": "Offer", price: "0", priceCurrency: "USD" },
      isAccessibleForFree: true,
      softwareVersion: p.version,
      downloadUrl: latestURL,
      license: licenseURL,
      screenshot: { "@type": "ImageObject", url: siteURL + p.shot.src, width: p.shot.w, height: p.shot.h, caption: t("shotLibraryAlt") },
      inLanguage: languages.map((l) => l.tag),
      author: { "@id": orgID },
      publisher: { "@id": orgID },
    },
    {
      "@type": "WebSite",
      "@id": home + "#website",
      url: home,
      name: "Kando",
      alternateName,
      description: t("metaDescription"),
      inLanguage: lang.tag,
      publisher: { "@id": orgID },
      about: { "@id": appID },
    },
  ];
  if (p.kind === "home") {
    graph.push({
      "@type": "FAQPage",
      "@id": home + "#faq",
      url: home,
      inLanguage: lang.tag,
      mainEntity: faqPairs.map(([q, a]) => ({ "@type": "Question", name: t(q), acceptedAnswer: { "@type": "Answer", text: t(a) } })),
    });
  } else {
    graph.push({
      "@type": "BreadcrumbList",
      "@id": pageURL(p.code, "sites") + "#breadcrumb",
      itemListElement: [
        { "@type": "ListItem", position: 1, name: brand, item: home },
        { "@type": "ListItem", position: 2, name: t("sitesPageTitle"), item: pageURL(p.code, "sites") },
      ],
    });
  }
  const res = graphSchema.safeParse({ "@context": schemaContext, "@graph": graph });
  if (!res.success) {
    throw new Error(res.error.issues.map((i) => `structured data of ${pageURL(p.code, p.kind)}: ${i.path.join(".")} ${i.message}`).join("\n"));
  }
  return res.data;
}
