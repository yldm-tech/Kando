// The page's words: its language's content strings (content/<code>.json) and the app's own strings in that language, which the hero's player shows so that it says what the app says.
import { createContext, Fragment, useContext, type ReactNode } from "react";
import type { AppKey, ContentModule } from "../../buildtime/schema";
import type en from "../../content/en.json";
import loaders from "virtual:kando/content-loaders";
import { language, type Language, type LangCode } from "../lib/site";

export type ContentKey = keyof typeof en;

// loadContent is a language's content, for a route's loader. Only the languages with a content file have a loader (buildtime/plugin.ts), and only they have pages.
export async function loadContent(code: LangCode): Promise<ContentModule> {
  const load = loaders[code];
  if (!load) {
    throw new Error(`no content for ${code}: content/${code}.json is missing`);
  }
  return (await load()).default;
}

export interface Words {
  lang: Language;
  content: ContentModule;
  // t is the content string key.
  t: (key: ContentKey) => string;
  // tf is the content string key with each {name} replaced by values' name, which fails on a name left unfilled.
  tf: (key: ContentKey, values: Record<string, string | number>) => string;
  // app is the app's string key, formatted as the page shows it.
  app: (key: AppKey) => string;
}

const WordsContext = createContext<Words | null>(null);

export function words(content: ContentModule): Words {
  const lang = language(content.code);
  const t = (key: ContentKey): string => {
    const s = content.strings[key];
    if (s === undefined) {
      throw new Error(`no content key ${key} in ${content.code}`);
    }
    return s;
  };
  return {
    lang,
    content,
    t,
    tf: (key, values) => {
      let s = t(key);
      for (const [name, value] of Object.entries(values)) {
        s = s.replaceAll("{" + name + "}", String(value));
      }
      const left = s.match(/\{[a-zA-Z]+\}/);
      if (left) {
        throw new Error(`${key}: ${left[0]} is not filled in`);
      }
      return s;
    },
    app: (key) => {
      const s = content.app[key];
      if (s === undefined) {
        throw new Error(`no app string ${key} in ${content.code}`);
      }
      return s;
    },
  };
}

export function WordsProvider({ content, children }: { content: ContentModule; children: ReactNode }) {
  return <WordsContext value={words(content)}>{children}</WordsContext>;
}

export function useWords(): Words {
  const w = useContext(WordsContext);
  if (!w) {
    throw new Error("useWords outside a WordsProvider");
  }
  return w;
}

// lines is s with each line break of the content string a <br>, where a heading breaks in the design.
function lines(s: string): ReactNode {
  const parts = s.split("\n");
  return parts.map((part, i) => (
    <Fragment key={i}>
      {i > 0 && <br />}
      {part}
    </Fragment>
  ));
}

// Rich is s with each pair of ` around a name, a file or a command set in <code>, as Kando's content strings mark them, and each line break a <br>.
export function Rich({ text }: { text: string }) {
  const parts = text.split("`");
  return <>{parts.map((part, i) => (i % 2 === 1 ? <code key={i}>{lines(part)}</code> : <Fragment key={i}>{lines(part)}</Fragment>))}</>;
}

// Headline is the hero's title: its first mark set apart, and each line break of the content string a <br>.
export function Headline({ title, mark }: { title: string; mark: string }) {
  const at = title.indexOf(mark);
  const before = at < 0 ? title : title.slice(0, at);
  const after = at < 0 ? "" : title.slice(at + mark.length);
  return (
    <>
      {lines(before)}
      <em className="mark">{lines(mark)}</em>
      {lines(after)}
    </>
  );
}
