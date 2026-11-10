// TestStructuredData: every page but 404.html carries one JSON-LD data block that the build's schemas accept: the app with the version of mygo.json, its publisher and the site in the page's language; a home page's FAQPage asks and answers what its FAQ section shows, and a sites page's BreadcrumbList leads back to its home page. The block is data, not script, so the page's policy names no hash for it.
import { existsSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vite-plus/test";
import { appVersion } from "../buildtime/app";
import { inlineScripts, scriptHash } from "../buildtime/csp";
import { appID, graphSchema, orgID, schemaContext, type Graph } from "../buildtime/jsonld";
import { language, latestURL, licenseURL, pageURL, repoURL, siteURL, sourceLang } from "../src/lib/site";
import { attr, builtLangs, dist, pageFile, readJSON, readOut, repoRoot, section, siteDir, tags, text } from "./helpers";

const ldRe = /<script type="application\/ld\+json">([\s\S]*?)<\/script>/g;

function graph(html: string): Graph {
  const blocks = [...html.matchAll(ldRe)].map((m) => m[1] ?? "");
  expect(blocks.length).toBe(1);
  return graphSchema.parse(JSON.parse(blocks[0] ?? ""));
}

function node<T extends Graph["@graph"][number]["@type"]>(g: Graph, type: T): Extract<Graph["@graph"][number], { "@type": T }> {
  const found = g["@graph"].filter((n) => n["@type"] === type);
  expect(found.length, type).toBe(1);
  return found[0] as Extract<Graph["@graph"][number], { "@type": T }>;
}

describe.each(["home", "sites"] as const)("%s pages", (kind) => {
  for (const l of builtLangs()) {
    it(l.code, () => {
      const html = readOut(pageFile(l, kind));
      const g = graph(html);
      const strings = readJSON(join(siteDir, "content", l.code + ".json"));
      expect(g["@context"]).toBe(schemaContext);

      const app = node(g, "SoftwareApplication");
      expect(app["@id"]).toBe(appID);
      expect(app.softwareVersion).toBe(appVersion(repoRoot));
      expect(app.downloadUrl).toBe(latestURL);
      expect(app.license).toBe(licenseURL);
      expect(app.description).toBe(strings["metaDescription"]);
      expect(app.alternateName).toBe(strings["brandName"] === "Kando" ? undefined : strings["brandName"]);
      expect(app.url).toBe(pageURL(l.code));
      expect(app.inLanguage).toEqual(builtLangs().map((o) => o.tag));
      for (const ref of [app.image, app.screenshot.url]) {
        expect(ref.startsWith(siteURL + "/"), ref).toBe(true);
        expect(existsSync(join(dist(), new URL(ref).pathname)), ref).toBe(true);
      }
      expect([app.author["@id"], app.publisher["@id"]]).toEqual([orgID, orgID]);
      expect(node(g, "Organization").sameAs).toEqual([repoURL]);

      const site = node(g, "WebSite");
      expect(site.inLanguage).toBe(l.tag);
      expect(site.url).toBe(pageURL(l.code));

      if (kind === "home") {
        // The questions and answers are the FAQ section's, as the page shows them.
        const faq = node(g, "FAQPage");
        const shown = [...section(html, 'class="faq-list"', "</section>").matchAll(/<summary[^>]*>([\s\S]*?)<\/summary>\s*<p>([\s\S]*?)<\/p>/g)].map((m) => ({
          q: text(m[1] ?? "").trim(),
          a: text(m[2] ?? "").trim(),
        }));
        expect(shown.length).toBeGreaterThan(0);
        expect(faq.mainEntity.map((e) => ({ q: e.name, a: e.acceptedAnswer.text }))).toEqual(shown);
        expect(faq.inLanguage).toBe(l.tag);
        expect(g["@graph"].some((n) => n["@type"] === "BreadcrumbList")).toBe(false);
      } else {
        const crumbs = node(g, "BreadcrumbList").itemListElement;
        expect(crumbs.map((c) => c.item)).toEqual([pageURL(l.code), pageURL(l.code, "sites")]);
        expect(crumbs.map((c) => c.name)).toEqual([strings["brandName"], strings["sitesPageTitle"]]);
        expect(g["@graph"].some((n) => n["@type"] === "FAQPage")).toBe(false);
      }

      // The data block is no script the policy allows by its hash.
      const meta = tags(html, "meta").find((t) => attr(t, "http-equiv") === "Content-Security-Policy");
      const csp = meta ? attr(meta, "content") : "";
      expect(csp).toContain("script-src 'self'");
      const block = [...html.matchAll(ldRe)][0]?.[1] ?? "";
      expect(inlineScripts(html)).not.toContain(block);
      expect(csp).not.toContain(scriptHash(block));
    });
  }
});

it("puts no structured data in 404.html", () => {
  expect(readOut("404.html")).not.toContain("application/ld+json");
});

it("rejects structured data that misses a field or has the wrong value", () => {
  const g = graph(readOut(pageFile(language(sourceLang))));
  const app = g["@graph"].find((n) => n["@type"] === "SoftwareApplication") ?? {};
  for (const change of [
    (a: Record<string, unknown>) => delete a["offers"],
    (a: Record<string, unknown>) => (a["softwareVersion"] = "latest"),
    (a: Record<string, unknown>) => (a["description"] = "Kando plays {count} sites"),
    (a: Record<string, unknown>) => (a["operatingSystem"] = "macOS"),
  ]) {
    const bad = structuredClone(app) as Record<string, unknown>;
    change(bad);
    expect(graphSchema.safeParse({ ...g, "@graph": g["@graph"].map((n) => (n["@type"] === "SoftwareApplication" ? bad : n)) }).success).toBe(false);
  }
});
