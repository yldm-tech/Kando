// SitesSearch is the catalog once the page runs: a search box over a table that draws only the rows on screen. The sites' aliases, which a search also matches, are fetched from webmcp/sites.json the first time the reader searches; until they arrive a search matches names and domains.
import site from "virtual:kando/site";
import { useQuery } from "@tanstack/react-query";
import { columnFilteringFeature, createFilteredRowModel, globalFilteringFeature, tableFeatures, useTable, type ColumnDef } from "@tanstack/react-table";
import { useWindowVirtualizer } from "@tanstack/react-virtual";
import { useMemo, useRef, useState } from "react";
import { z } from "zod/mini";
import { useWords } from "../../i18n/content";
import { siteMatches } from "../../lib/match";
import { SiteRow, SitesHead, TableCard, tableClass } from "./SitesPage";
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
      <div className="mb-5 flex flex-wrap items-center gap-x-4 gap-y-2.5" role="search">
        <label htmlFor="sites-query" className="sr">
          {t("sitesSearchLabel")}
        </label>
        <span className="relative flex min-w-0 flex-[1_1_320px] items-center">
          <svg className="pointer-events-none absolute left-4 size-[18px] fill-none stroke-ink-3 stroke-2" viewBox="0 0 24 24" aria-hidden="true">
            <circle cx="11" cy="11" r="7" />
            <path d="m20 20-3.5-3.5" strokeLinecap="round" />
          </svg>
          <input
            id="sites-query"
            type="search"
            className="h-12 w-full rounded-full border-0 bg-white pr-5 pl-11 text-[15px] text-ink shadow-[0_0_0_1px_rgba(28,25,22,0.1),0_8px_24px_rgba(80,40,10,0.06)] transition-shadow duration-300 outline-none placeholder:text-ink-3 focus:shadow-[0_0_0_2px_var(--color-accent),0_8px_24px_rgba(229,101,29,0.14)]"
            value={query}
            placeholder={t("sitesSearchPlaceholder")}
            autoComplete="off"
            spellCheck={false}
            onFocus={() => setWanted(true)}
            onChange={(e) => setQuery(e.target.value)}
          />
        </span>
        <output htmlFor="sites-query" aria-live="polite" className="text-[13px] text-ink-3 tabular-nums">
          {tf("sitesFound", { count: found.length })}
        </output>
      </div>
      <TableCard>
        <table className={tableClass} aria-labelledby="sites-title" aria-rowcount={found.length + 1}>
          <SitesHead />
          <tbody ref={body}>
            {before > 0 && (
              <tr aria-hidden="true">
                <td colSpan={2} className="p-0" style={{ height: before }} />
              </tr>
            )}
            {items.map((item) => {
              const row = found[item.index];
              // The header is the table's first row, so the sites are numbered from 2 among all found, wherever the reader has scrolled.
              return row ? <SiteRow key={row.original.domain} row={row.original} rowIndex={item.index + 2} /> : null;
            })}
            {after > 0 && (
              <tr aria-hidden="true">
                <td colSpan={2} className="p-0" style={{ height: after }} />
              </tr>
            )}
          </tbody>
        </table>
        {found.length === 0 && <p className="m-0 px-4 py-10 text-center text-[15px] text-ink-3">{t("sitesNoMatch")}</p>}
      </TableCard>
    </>
  );
}
