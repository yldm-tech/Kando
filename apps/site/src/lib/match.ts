// siteMatches is the app's catalog.Site.Matches, as static/webmcp.js has it for search_supported_sites: a link finds the site whose domain, or a domain among its aliases, is its host or a parent of it; other text finds a site when every word of it, ignoring case, is part of its name, its domain or one of its aliases; an empty query finds every site.

export interface MatchSite {
  name: string;
  domain: string;
  aliases?: readonly string[];
}

// linkHost is the host of query when it is a link to a page, with or without its scheme (a host then needs a path after it); "" otherwise. It is apps/kando/sites/catalog's linkHost.
export function linkHost(query: string): string {
  let q = query.trim().toLowerCase();
  if (/[ \t]/.test(q) || !q.includes(".")) {
    return "";
  }
  if (!q.includes("://")) {
    if (!q.includes("/")) {
      return "";
    }
    q = "https://" + q;
  }
  try {
    return new URL(q).hostname.replace(/\.$/, "");
  } catch {
    return "";
  }
}

export function siteMatches(site: MatchSite, query: string): boolean {
  const domains = [site.domain, ...(site.aliases ?? [])];
  const host = linkHost(query);
  if (host) {
    return domains.some((domain) => domain.includes(".") && (host === domain || host.endsWith("." + domain)));
  }
  const words = query.toLowerCase().split(/\s+/).filter(Boolean);
  const hay = (site.name + " " + domains.join(" ")).toLowerCase();
  return words.every((word) => hay.includes(word));
}
