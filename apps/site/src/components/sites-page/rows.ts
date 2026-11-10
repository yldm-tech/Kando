import type { CatalogRow } from "../../../buildtime/schema";
import type { Language } from "../../lib/site";

// Row is a website as the page in a language lists it: with the name that language writes, and a name in Chinese characters on a page not in Chinese marked as Simplified Chinese, the catalog's script.
export interface Row extends CatalogRow {
  lang?: string;
}

const hanRe = /\p{Script=Han}/u;

export function localRows(rows: readonly CatalogRow[], lang: Language, names: Record<string, string>): Row[] {
  return rows.map((r) => {
    const name = names[r.domain] ?? r.name;
    const row: Row = { ...r, name };
    if (!lang.tag.startsWith("zh") && hanRe.test(name)) {
      row.lang = "zh-Hans";
    }
    return row;
  });
}
