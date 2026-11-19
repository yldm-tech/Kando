// PluginDirectory is the list of community plugins once the page runs: it fetches the plugin index (lib/plugins.ts), then filters it by a search and a kind. Rendered on the server, and until the index arrives, it says the list is loading; it says so too when the browser is offline or the index could not be read, with a button to try again. Every field of the index is shown as text, never as markup.
import { useQuery } from "@tanstack/react-query";
import { useHydrated } from "@tanstack/react-router";
import { useEffect, useMemo, useState, type ReactNode } from "react";
import { useWords, type ContentKey } from "../../i18n/content";
import {
  fetchIndex,
  filterPlugins,
  formatSize,
  installLink,
  languageName,
  localText,
  OfflineError,
  pluginKinds,
  type PluginEntry,
  type PluginKind,
} from "../../lib/plugins";

// kindKeys are the content keys of the kinds' names.
const kindKeys: Record<PluginKind, ContentKey> = {
  dictionary: "pluginKindDictionary",
  wordlist: "pluginKindWordlist",
  glossary: "pluginKindGlossary",
  "ai-service": "pluginKindAiService",
  "translate-style": "pluginKindTranslateStyle",
  task: "pluginKindTask",
  "export-template": "pluginKindExportTemplate",
  site: "pluginKindSite",
  script: "pluginKindScript",
};

const fieldClass =
  "h-12 rounded-full border-0 bg-white text-[15px] text-ink shadow-[0_0_0_1px_rgba(28,25,22,0.1),0_8px_24px_rgba(80,40,10,0.06)] transition-shadow duration-300 outline-none focus:shadow-[0_0_0_2px_var(--color-accent),0_8px_24px_rgba(229,101,29,0.14)]";

export function PluginDirectory() {
  const { t } = useWords();
  const hydrated = useHydrated();
  // The index is fetched once the page runs, never while it is prerendered. It is fetched whether or not the browser thinks it is online, so that being offline shows as such rather than as a list that never loads.
  const index = useQuery({
    queryKey: ["plugin-index"],
    queryFn: ({ signal }) => fetchIndex(signal),
    enabled: hydrated,
    retry: false,
    networkMode: "always",
    staleTime: Infinity,
  });
  const { refetch, isError } = index;

  // Coming back online after a failed fetch tries again by itself.
  useEffect(() => {
    if (!isError) {
      return;
    }
    const again = () => void refetch();
    window.addEventListener("online", again);
    return () => window.removeEventListener("online", again);
  }, [isError, refetch]);

  if (index.isError) {
    const offline = index.error instanceof OfflineError;
    return (
      <Notice role="alert">
        <p>{t(offline ? "pluginOffline" : "pluginError")}</p>
        <button type="button" className="btn ghost mt-5" onClick={() => void refetch()} disabled={index.isFetching}>
          {t("pluginRetry")}
        </button>
      </Notice>
    );
  }
  if (!index.isSuccess) {
    return (
      <Notice role="status">
        <p>{t("pluginLoading")}</p>
      </Notice>
    );
  }
  if (index.data.entries.length === 0) {
    return (
      <Notice role="status">
        <p>{t("pluginEmpty")}</p>
      </Notice>
    );
  }
  return <PluginList entries={index.data.entries} />;
}

function Notice({ role, children }: { role: "status" | "alert"; children: ReactNode }) {
  return (
    <div className="plugin-notice card rounded-[22px] px-6 py-12 text-center text-[15px] text-ink-2" role={role}>
      {children}
    </div>
  );
}

function PluginList({ entries }: { entries: readonly PluginEntry[] }) {
  const { t, tf } = useWords();
  const [query, setQuery] = useState("");
  const [kind, setKind] = useState<PluginKind | "">("");
  // The kind menu offers only the kinds the index has, in the app's order.
  const kinds = useMemo(() => pluginKinds.filter((k) => entries.some((e) => e.kind === k)), [entries]);
  const found = useMemo(() => filterPlugins(entries, query, kind), [entries, query, kind]);
  return (
    <>
      <div className="mb-5 flex flex-wrap items-center gap-x-4 gap-y-2.5" role="search">
        <label htmlFor="plugin-query" className="sr">
          {t("pluginSearchLabel")}
        </label>
        <span className="relative flex min-w-0 flex-[1_1_320px] items-center">
          <svg className="pointer-events-none absolute left-4 size-[18px] fill-none stroke-ink-3 stroke-2" viewBox="0 0 24 24" aria-hidden="true">
            <circle cx="11" cy="11" r="7" />
            <path d="m20 20-3.5-3.5" strokeLinecap="round" />
          </svg>
          <input
            id="plugin-query"
            type="search"
            className={fieldClass + " w-full pr-5 pl-11 placeholder:text-ink-3"}
            value={query}
            placeholder={t("pluginSearchPlaceholder")}
            autoComplete="off"
            spellCheck={false}
            maxLength={200}
            onChange={(e) => setQuery(e.target.value)}
          />
        </span>
        <label htmlFor="plugin-kind" className="sr">
          {t("pluginKindLabel")}
        </label>
        <select id="plugin-kind" className={fieldClass + " px-5"} value={kind} onChange={(e) => setKind(kindOf(e.target.value))}>
          <option value="">{t("pluginKindAll")}</option>
          {kinds.map((k) => (
            <option key={k} value={k}>
              {t(kindKeys[k])}
            </option>
          ))}
        </select>
        <output htmlFor="plugin-query plugin-kind" aria-live="polite" className="text-[13px] text-ink-3 tabular-nums">
          {tf("pluginFound", { count: found.length })}
        </output>
      </div>
      {found.length === 0 ? (
        <Notice role="status">
          <p>{t("pluginNoMatch")}</p>
        </Notice>
      ) : (
        <ul className="plugin-list grid grid-cols-[repeat(auto-fill,minmax(300px,1fr))] gap-4" aria-labelledby="plugin-title">
          {found.map((e) => (
            <PluginCard key={e.id} entry={e} />
          ))}
        </ul>
      )}
    </>
  );
}

function kindOf(v: string): PluginKind | "" {
  return pluginKinds.find((k) => k === v) ?? "";
}

const badgeClass = "inline-flex h-6 shrink-0 items-center rounded-full px-2.5 text-[12px] font-semibold whitespace-nowrap";

function PluginCard({ entry: e }: { entry: PluginEntry }) {
  const { t, lang } = useWords();
  const name = localText(e.name, lang.tag);
  const summary = localText(e.summary, lang.tag);
  // A text in another language than the page's says so, for its font and for screen readers.
  const langOf = (tag: string | undefined) => (tag === undefined || tag.toLowerCase() === lang.tag.toLowerCase() ? undefined : tag);
  return (
    <li className="plugin-card card flex min-w-0 flex-col gap-3 rounded-[22px] p-5">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <h3 lang={langOf(name?.tag)} className="text-[17px] leading-snug font-bold break-words">
            {name?.text ?? e.id}
          </h3>
          <p className="mt-0.5 truncate font-mono text-[12px] text-ink-3">{e.id}</p>
        </div>
        <span className={badgeClass + " bg-[rgba(28,25,22,0.06)] text-ink-2"}>{t(kindKeys[e.kind])}</span>
      </div>
      {e.source && (
        <span className={badgeClass + " self-start bg-accent/[0.1] text-accent-ink"}>{t(e.source === "official" ? "pluginOfficial" : "pluginVerified")}</span>
      )}
      {summary && (
        <p lang={langOf(summary.tag)} className="text-[14px] leading-[1.6] break-words text-ink-2">
          {summary.text}
        </p>
      )}
      <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-1 text-[13px]">
        <Field label={t("pluginPublisher")}>{e.publisher}</Field>
        <Field label={t("pluginVersion")}>{e.version}</Field>
        <Field label={t("pluginSize")}>{formatSize(e.size, lang.tag)}</Field>
        <Field label={t("pluginLicense")}>{e.license}</Field>
        {e.languages.length > 0 && <Field label={t("pluginLanguages")}>{e.languages.map((l) => languageName(l, lang.tag)).join(", ")}</Field>}
      </dl>
      <div className="mt-auto flex flex-wrap items-center gap-x-4 gap-y-2 pt-2">
        <a className="btn primary h-10 rounded-[20px] px-[18px] text-[14px]" href={installLink(e.id)}>
          {t("pluginOpen")}
        </a>
        {e.homepage && (
          <a className="text-[14px] font-semibold text-accent-ink hover:text-accent-hover" href={e.homepage} rel="noopener noreferrer nofollow ugc">
            {t("pluginHomepage")}
          </a>
        )}
      </div>
    </li>
  );
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <>
      <dt className="text-ink-3">{label}</dt>
      <dd className="m-0 break-words text-ink-list">{children}</dd>
    </>
  );
}
