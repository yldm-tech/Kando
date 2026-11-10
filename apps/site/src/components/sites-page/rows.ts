import type { CatalogRow } from "../../../buildtime/schema";
import { initial, type Language } from "../../lib/site";

// Row is a website as the page in a language lists it: with the name that language writes and that name's initial, and a name in Chinese characters on a page not in Chinese marked as Simplified Chinese, the catalog's script.
export interface Row extends CatalogRow {
  lang?: string;
  // aliases are yt-dlp's names for the site, which a search matches too; the page has them once it has fetched webmcp/sites.json.
  aliases?: string[];
}

const hanRe = /\p{Script=Han}/u;

export function localRows(rows: readonly CatalogRow[], lang: Language, names: Record<string, string>): Row[] {
  return rows.map((r) => {
    const name = names[r.domain] ?? r.name;
    const row: Row = { ...r, name };
    // A site without a logo shows the first letter of the name the page writes, as the logo strip does.
    if (name !== r.name && row.initial !== undefined) {
      row.initial = initial(name);
    }
    if (!lang.tag.startsWith("zh") && hanRe.test(name)) {
      row.lang = "zh-Hans";
    }
    return row;
  });
}
