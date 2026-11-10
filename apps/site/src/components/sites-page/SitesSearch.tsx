// SitesSearch is the catalog once the page runs: a search box over a table that draws only the rows on screen. The sites' aliases, which a search also matches, are fetched from webmcp/sites.json the first time the reader searches; until they arrive a search matches names and domains.
import site from "virtual:kando/site";
import { useQuery } from "@tanstack/react-query";
import { columnFilteringFeature, createFilteredRowModel, globalFilteringFeature, tableFeatures, useTable, type ColumnDef } from "@tanstack/react-table";
import { useWindowVirtualizer } from "@tanstack/react-virtual";
import { useMemo, useRef, useState } from "react";
import { z } from "zod/mini";
import { useWords } from "../../i18n/content";
import { siteMatches } from "../../lib/match";
import { SiteRow, SitesHead } from "./SitesPage";
import type { Row } from "./rows";

// sitesFileSchema is webmcp/sites.json, as buildtime/webmcp.ts writes it.
const sitesFileSchema = z.object({
  ytdlp: z.string(),
  sites: z.array(z.object({ name: z.string(), domain: z.string(), aliases: z.optional(z.array(z.string())) })),
});

async function fetchAliases(url: string): Promise<Map<string, string[]>> {
  const res = await fetch(url);
  if (!res.ok) {
    throw new Error(`HTTP ${res.status} for ${url}`);
  }
  const file = sitesFileSchema.parse(await res.json());
  return new Map(file.sites.map((s) => [s.domain, s.aliases ?? []]));
}

const features = tableFeatures({
  columnFilteringFeature,
  globalFilteringFeature,
  filteredRowModel: createFilteredRowModel(),
});

export function SitesSearch({ rows }: { rows: readonly Row[] }) {
  const { t, tf } = useWords();
  const [query, setQuery] = useState("");
  const [wanted, setWanted] = useState(false);
  const aliases = useQuery({
    queryKey: ["sites", site.assets.sitesJSON],
    queryFn: () => fetchAliases(site.assets.sitesJSON),
    enabled: wanted || query !== "",
    staleTime: Infinity,
  });
  const known = aliases.data;
  // The rows with their aliases once they arrive, so that the table filters again with them.
  const data = useMemo(() => (known ? rows.map((r) => ({ ...r, aliases: known.get(r.domain) ?? [] })) : [...rows]), [rows, known]);
  const columns = useMemo<ColumnDef<typeof features, Row>[]>(
    () => [
      { id: "name", accessorKey: "name", header: () => t("sitesColumnName") },
      { id: "domain", accessorKey: "domain", header: () => t("sitesColumnDomain") },
    ],
    [t],
  );
  const table = useTable({
    features,
    columns,
    data,
    state: { globalFilter: query },
    // One column takes part, and its filter matches the whole site as the app does, so that every word of a query may be in a different field.
    getColumnCanGlobalFilter: (column) => column.id === "name",
    globalFilterFn: (row, _columnId, value: string) => siteMatches(row.original, value),
  });
  const found = table.getRowModel().rows;

  const body = useRef<HTMLTableSectionElement>(null);
  const virtualizer = useWindowVirtualizer({
    count: found.length,
    estimateSize: () => 44,
    overscan: 8,
    scrollMargin: body.current?.offsetTop ?? 0,
  });
  const items = virtualizer.getVirtualItems();
  const before = items[0] ? items[0].start - virtualizer.options.scrollMargin : 0;
  const after = items.length > 0 ? virtualizer.getTotalSize() - ((items[items.length - 1]?.end ?? 0) - virtualizer.options.scrollMargin) : 0;

  return (
    <>
      <div className="sites-search" role="search">
        <label htmlFor="sites-query">{t("sitesSearchLabel")}</label>
        <input
          id="sites-query"
          type="search"
          value={query}
          placeholder={t("sitesSearchPlaceholder")}
          autoComplete="off"
          spellCheck={false}
          onFocus={() => setWanted(true)}
          onChange={(e) => setQuery(e.target.value)}
        />
        <output htmlFor="sites-query" aria-live="polite">
          {found.length === 0 ? t("sitesNoMatch") : tf("sitesFound", { count: found.length })}
        </output>
      </div>
      <table className="sites-table" aria-labelledby="sites-title" aria-rowcount={found.length + 1}>
        <SitesHead />
        <tbody ref={body}>
          {before > 0 && (
            <tr className="spacer" aria-hidden="true">
              <td colSpan={2} style={{ height: before }} />
            </tr>
          )}
          {items.map((item) => {
            const row = found[item.index];
            return row ? <SiteRow key={row.original.domain} row={row.original} /> : null;
          })}
          {after > 0 && (
            <tr className="spacer" aria-hidden="true">
              <td colSpan={2} style={{ height: after }} />
            </tr>
          )}
        </tbody>
      </table>
    </>
  );
}
