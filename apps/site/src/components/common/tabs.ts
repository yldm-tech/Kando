// useTabs makes a group of tabs: the tab list's buttons and their panels, one shown at a time, with the arrow keys moving between the tabs as a tab list's keys do.
import { useHydrated } from "@tanstack/react-router";
import { useCallback, useState, type KeyboardEvent } from "react";

export interface Tabs {
  selected: number;
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
  // stack is the element the panels go in, all of them and nothing else.
  stack: { className: string; "data-moved"?: "" };
  panel: (i: number) => { role: "tabpanel"; id: string; "aria-labelledby": string; "data-off"?: "" };
}

// ids are each tab's id and its panel's. The panels go in one element, the stack, which the stylesheet makes a grid of one cell once the head script has marked the page as running script: every panel lies in that cell and the panels but the chosen one, marked data-off, are invisible there, so that the stack is as tall as its tallest panel from the first paint, before and after hydration, and choosing another tab never changes the section's height. Once the reader has chosen a tab the stack is marked data-moved, and the stylesheet fades the panel that leaves and brings in the one that arrives (k-panel); the first panel is simply there. Without script every panel shows, one after another, as the page reads complete without it; the player's side panel shows only the first, as it has room for one (the stylesheet's .kp-pane rules).
export function useTabs(ids: readonly (readonly [string, string])[]): Tabs {
  const [selected, setSelected] = useState(0);
  const [moved, setMoved] = useState(false);
  const hydrated = useHydrated();
  const select = useCallback((i: number) => {
    setSelected(i);
    setMoved(true);
  }, []);
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
    stack: { className: "tab-stack", ...(moved ? { "data-moved": "" as const } : {}) },
    panel: (i) => ({
      role: "tabpanel",
      id: ids[i]?.[1] ?? "",
      "aria-labelledby": ids[i]?.[0] ?? "",
      ...(i !== selected ? { "data-off": "" as const } : {}),
    }),
  };
}
