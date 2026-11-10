// useTabs makes a group of tabs: the tab list's buttons and their panels, one shown at a time, with the arrow keys moving between the tabs as a tab list's keys do.
import { useHydrated } from "@tanstack/react-router";
import { useEffect, useRef, useState, type KeyboardEvent } from "react";
import { arrive } from "./motion";

export interface Tabs {
  selected: number;
  hydrated: boolean;
  select: (i: number) => void;
  tab: (i: number) => {
    type: "button";
    role: "tab";
    id: string;
    "aria-controls": string;
    "aria-selected": boolean;
    tabIndex?: number;
    onClick: () => void;
    onKeyDown: (e: KeyboardEvent) => void;
  };
  panel: (i: number) => { role: "tabpanel"; id: string; "aria-labelledby": string; hidden: boolean };
}

// ids are each tab's id and its panel's. Without script every panel shows, as the page reads complete without it, and the stylesheet lays them out one after another; hideBeforeHydration hides the other panels from the start instead, for the player's side panel, which has room for one.
export function useTabs(ids: readonly (readonly [string, string])[], opts: { hideBeforeHydration?: boolean } = {}): Tabs {
  const [selected, select] = useState(0);
  const hydrated = useHydrated();
  const hiding = hydrated || opts.hideBeforeHydration === true;
  // A panel the reader switches to arrives with a short rise; the first one is simply there.
  const shown = useRef(selected);
  useEffect(() => {
    if (shown.current !== selected) {
      shown.current = selected;
      arrive(document.getElementById(ids[selected]?.[1] ?? ""));
    }
  }, [selected, ids]);
  const move = (e: KeyboardEvent, i: number) => {
    if (e.key !== "ArrowRight" && e.key !== "ArrowLeft") {
      return;
    }
    const next = (i + (e.key === "ArrowRight" ? 1 : ids.length - 1)) % ids.length;
    select(next);
    document.getElementById(ids[next]?.[0] ?? "")?.focus();
    e.preventDefault();
  };
  return {
    selected,
    hydrated,
    select,
    tab: (i) => ({
      type: "button",
      role: "tab",
      id: ids[i]?.[0] ?? "",
      "aria-controls": ids[i]?.[1] ?? "",
      "aria-selected": i === selected,
      tabIndex: hydrated ? (i === selected ? 0 : -1) : undefined,
      onClick: () => select(i),
      onKeyDown: (e) => move(e, i),
    }),
    panel: (i) => ({ role: "tabpanel", id: ids[i]?.[1] ?? "", "aria-labelledby": ids[i]?.[0] ?? "", hidden: hiding && i !== selected }),
  };
}
