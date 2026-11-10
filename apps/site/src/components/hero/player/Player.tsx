// The hero's player, after the design: the demo film playing with its two lines of subtitles, the controls and the side panel of the app's player, in the app's own words. It plays as the app's player plays a film: the line and the scene follow the time, and every control does what it does in the app. Rendered without script it shows the film's first line, still; it starts paused where the reader asked for less motion.
import { useEffect, useLayoutEffect, useRef, useState, type KeyboardEvent, type MouseEvent } from "react";
import { useReducedMotion } from "motion/react";
import { useWords, type ContentKey } from "../../../i18n/content";
import { clock, demoLineSeconds, demoLines, demoSeconds } from "../../../lib/site";
import { SlidingPill } from "../../common/SlidingPill";
import { useTabs } from "../../common/tabs";
import { BackIcon, BigPlayIcon, FullScreenIcon, LoopIcon, ModeIcon, PanelIcon, PauseIcon, PlayIcon, SkipBackIcon, SkipForwardIcon, SoundIcon } from "./icons";
import { Scene } from "./scenes";

// cueKeys are the translations of demoLines, in the same order.
const cueKeys: readonly ContentKey[] = ["demoCue1", "demoCue2", "demoCue3", "demoCue4", "demoCue5", "demoCue6", "demoCue7", "demoCue8"];

const clipCount = demoLines.length / 2;

type Size = "s" | "m" | "l";
type Order = "translation" | "original";

// share is where on el, from 0 to 1, a click landed.
function share(event: MouseEvent, el: Element): number {
  const box = el.getBoundingClientRect();
  return Math.max(0, Math.min(1, (event.clientX - box.left) / box.width));
}

export function Player() {
  const { t, app, content } = useWords();
  const reduced = useReducedMotion();
  const cues = demoLines.map((original, i) => ({ original, translation: t(cueKeys[i] ?? "demoCue1") }));
  const modeLabels = [app("filmModeBilingual"), app("filmModeOriginal"), app("filmModeTranslation"), app("filmModeOff")];

  const [time, setTime] = useState(0);
  const [playing, setPlaying] = useState(true);
  const [loop, setLoop] = useState(-1);
  const [mode, setMode] = useState(0);
  const [volume, setVolume] = useState(0.8);
  const [muted, setMuted] = useState(false);
  const [size, setSize] = useState<Size>("m");
  const [order, setOrder] = useState<Order>("translation");
  const [panel, setPanel] = useState(true);
  const [query, setQuery] = useState("");
  const tabs = useTabs(
    [
      ["kp-tab-subs", "kp-subs"],
      ["kp-tab-clips", "kp-clips"],
      ["kp-tab-info", "kp-info"],
      ["kp-tab-style", "kp-style"],
    ],
    { hideBeforeHydration: true },
  );

  const root = useRef<HTMLDivElement>(null);
  const seekFill = useRef<HTMLSpanElement>(null);
  const seekKnob = useRef<HTMLSpanElement>(null);
  const volumeFill = useRef<HTMLSpanElement>(null);
  const volumeKnob = useRef<HTMLSpanElement>(null);

  const line = Math.min(cues.length - 1, Math.floor(time / demoLineSeconds));
  const cue = cues[line] ?? cues[0];
  const level = muted ? 0 : volume;

  // Where the reader asked for less motion, the film waits to be played.
  useEffect(() => {
    if (reduced) {
      setPlaying(false);
    }
  }, [reduced]);

  // loopRef is loop for the clock, which keeps the line it loops.
  const loopRef = useRef(loop);
  useEffect(() => {
    loopRef.current = loop;
  }, [loop]);
  useEffect(() => {
    if (!playing) {
      return;
    }
    const id = window.setInterval(() => {
      setTime((now) => {
        const l = loopRef.current;
        let next = now + 0.25;
        if (l >= 0 && (next >= (l + 1) * demoLineSeconds || next < l * demoLineSeconds)) {
          next = l * demoLineSeconds;
        }
        return next >= demoSeconds ? 0 : next;
      });
    }, 250);
    return () => window.clearInterval(id);
  }, [playing]);

  // The bars' fills and knobs move through the style object, after rendering, so that the server's HTML has no style attribute.
  useLayoutEffect(() => {
    const at = (time / demoSeconds) * 100 + "%";
    seekFill.current?.style.setProperty("width", at);
    seekKnob.current?.style.setProperty("left", at);
  }, [time]);
  useLayoutEffect(() => {
    const at = level * 100 + "%";
    volumeFill.current?.style.setProperty("width", at);
    volumeKnob.current?.style.setProperty("left", at);
  }, [level]);

  const seekTo = (seconds: number) => {
    const next = Math.max(0, Math.min(demoSeconds - 0.01, seconds));
    setTime(next);
    if (loop >= 0) {
      setLoop(Math.min(cues.length - 1, Math.floor(next / demoLineSeconds)));
    }
  };
  const setLevel = (l: number, mute: boolean) => {
    setVolume(l);
    setMuted(mute || l === 0);
  };
  const onSliderKey = (event: KeyboardEvent, which: "seek" | "volume") => {
    const step = event.key === "ArrowRight" || event.key === "ArrowUp" ? 1 : event.key === "ArrowLeft" || event.key === "ArrowDown" ? -1 : 0;
    if (!step) {
      return;
    }
    if (which === "seek") {
      seekTo(time + step * 5);
    } else {
      setLevel(Math.max(0, Math.min(1, level + step * 0.1)), false);
    }
    event.preventDefault();
  };
  const fullScreen = () => {
    const win = root.current?.closest(".win") ?? root.current;
    if (document.fullscreenElement) {
      void document.exitFullscreen();
    } else if (win?.requestFullscreen) {
      void win.requestFullscreen();
    }
  };

  const q = query.trim().toLowerCase();
  const hits = cues.map((c) => !q || (c.translation + " " + c.original).toLowerCase().includes(q));
  const playLabel = playing ? app("filmPause") : app("filmPlay");
  const muteLabel = muted ? app("filmUnmute") : app("filmMute");
  const classes = ["kp", playing ? "" : "paused", muted ? "muted" : "", loop >= 0 ? "looping" : "", panel ? "" : "no-panel"].filter(Boolean).join(" ");

  return (
    <div className={classes} id="player" ref={root} data-mode={mode} data-size={size} data-order={order}>
      <div className="kp-main">
        <div className="kp-head">
          <span className="kp-back" aria-hidden="true">
            <BackIcon />
          </span>
          <span className="kp-title">
            <strong lang="en">How Rivers Shape Landscapes</strong>
            <span>{t("demoLibrary") + " · " + app("filmStatusTranslated")}</span>
          </span>
          <button
            className="kp-icon"
            type="button"
            aria-pressed={panel}
            aria-label={app("filmSidebar")}
            title={app("filmSidebar")}
            onClick={() => setPanel(!panel)}
          >
            <PanelIcon />
          </button>
        </div>
        <div className="kp-picture" onClick={() => setPlaying(!playing)}>
          {Array.from({ length: clipCount }, (_, i) => (
            <div key={i} className={"kp-scene" + (i === Math.floor(line / 2) ? " on" : "")} data-scene={i} aria-hidden="true">
              <Scene n={i} id={"kp-sc" + i} />
            </div>
          ))}
          <div className="kp-clouds" aria-hidden="true">
            <span />
            <span />
          </div>
          <span className="kp-big" aria-hidden="true">
            <BigPlayIcon />
          </span>
          <span className="kp-looping" aria-hidden="true">
            {app("filmLoopOn")}
          </span>
          <div className="kp-subs" aria-live="off">
            <span className="kp-tr">{cue?.translation}</span>
            <span className="kp-or" lang="en">
              {cue?.original}
            </span>
          </div>
        </div>
        <div className="kp-controls">
          <div className="kp-bar">
            <button
              className="kp-btn"
              type="button"
              data-act="back"
              aria-label={app("filmSkipBack")}
              title={app("filmSkipBack")}
              onClick={() => seekTo(time - 5)}
            >
              <SkipBackIcon />
            </button>
            <button className="kp-btn kp-play" type="button" aria-label={playLabel} title={playLabel} onClick={() => setPlaying(!playing)}>
              <PauseIcon />
              <PlayIcon />
            </button>
            <button
              className="kp-btn"
              type="button"
              data-act="forward"
              aria-label={app("filmSkipForward")}
              title={app("filmSkipForward")}
              onClick={() => seekTo(time + 5)}
            >
              <SkipForwardIcon />
            </button>
            <span className="kp-time kp-now">{clock(time)}</span>
            <div
              className="kp-seek"
              role="slider"
              tabIndex={0}
              aria-label={app("filmPlaybackPosition")}
              aria-valuemin={0}
              aria-valuemax={demoSeconds}
              aria-valuenow={Math.floor(time)}
              onClick={(e) => seekTo(share(e, e.currentTarget) * demoSeconds)}
              onKeyDown={(e) => onSliderKey(e, "seek")}
            >
              <span className="kp-track" />
              <span className="kp-fill" ref={seekFill} />
              <span className="kp-knob" ref={seekKnob} />
              <span className="kp-segs">
                {cues.map((_, i) => (
                  <i key={i} className={i < line ? "done" : i === line ? "now" : undefined} />
                ))}
              </span>
            </div>
            <span className="kp-time kp-left">{"-" + clock(demoSeconds - time)}</span>
            <button
              className="kp-icon kp-mute"
              type="button"
              aria-pressed={muted}
              aria-label={muteLabel}
              title={muteLabel}
              onClick={() => setLevel(volume || 0.8, !muted)}
            >
              <SoundIcon />
            </button>
            <div
              className="kp-volume"
              role="slider"
              tabIndex={0}
              aria-label={app("filmVolume")}
              aria-valuemin={0}
              aria-valuemax={100}
              aria-valuenow={Math.round(level * 100)}
              onClick={(e) => setLevel(share(e, e.currentTarget), false)}
              onKeyDown={(e) => onSliderKey(e, "volume")}
            >
              <span className="kp-track" />
              <span className="kp-fill" ref={volumeFill} />
              <span className="kp-knob" ref={volumeKnob} />
            </div>
            <button
              className="kp-icon"
              type="button"
              data-act="loop"
              aria-pressed={loop >= 0}
              aria-label={app("filmLoopLine")}
              title={app("filmLoopLine")}
              onClick={() => setLoop(loop >= 0 ? -1 : line)}
            >
              <LoopIcon />
            </button>
            <button className="kp-mode" type="button" title={app("filmModeHeading")} onClick={() => setMode((mode + 1) % modeLabels.length)}>
              <ModeIcon />
              <span>{modeLabels[mode]}</span>
            </button>
            <button
              className="kp-icon on"
              type="button"
              aria-pressed={panel}
              aria-label={app("filmSidebar")}
              title={app("filmSidebar")}
              onClick={() => setPanel(!panel)}
            >
              <PanelIcon />
            </button>
            <button
              className="kp-icon"
              type="button"
              data-act="fullscreen"
              aria-label={app("filmFullScreen")}
              title={app("filmFullScreen")}
              onClick={fullScreen}
            >
              <FullScreenIcon />
            </button>
          </div>
        </div>
      </div>
      <aside className="kp-side" aria-label={app("filmSidebar")}>
        <div className="kp-card">
          <div className="kp-tabs" role="tablist" aria-label={app("filmSidebar")}>
            <SlidingPill index={tabs.selected} items='[role="tab"]' className="rounded-[8px] bg-white/16" />
            {(["filmSubtitles", "filmTabClips", "filmTabInfo", "filmTabStyle"] as const).map((key, i) => (
              <button key={key} {...tabs.tab(i)}>
                {app(key)}
              </button>
            ))}
          </div>
          <div className="kp-pane kp-lines-pane" {...tabs.panel(0)}>
            <input
              className="kp-search"
              type="search"
              aria-label={app("filmSearchLines")}
              placeholder={app("filmSearchLines")}
              value={query}
              onChange={(e) => setQuery(e.target.value)}
            />
            <div className="kp-lines">
              {cues.map((c, i) => (
                <button
                  key={i}
                  type="button"
                  className={"kp-line" + (i === line ? " on" : "")}
                  data-line={i + 1}
                  title={app("filmClickToJump")}
                  hidden={!hits[i]}
                  onClick={() => seekTo(i * demoLineSeconds + 0.05)}
                >
                  <span className="kp-at">{clock(i * demoLineSeconds)}</span>
                  <span className="kp-text">
                    <span>{c.translation}</span>
                    <span lang="en">{c.original}</span>
                  </span>
                </button>
              ))}
              <p className="kp-none" hidden={hits.some(Boolean)}>
                {app("filmNoMatch")}
              </p>
            </div>
          </div>
          <div className="kp-pane" {...tabs.panel(1)}>
            <p className="kp-note">{app("filmClipsNote")}</p>
            {content.clipSpans.map((span, k) => (
              <button
                key={k}
                type="button"
                className={"kp-clip" + (k === Math.floor(line / 2) ? " on" : "")}
                data-clip={k + 1}
                onClick={() => {
                  seekTo(k * 2 * demoLineSeconds + 0.05);
                  setPlaying(true);
                }}
              >
                <span className="kp-thumb">
                  <Scene n={k} id={"kp-th" + k} />
                </span>
                <span className="kp-text">
                  <span className="kp-span">{span}</span>
                  <span>{cues[2 * k]?.translation}</span>
                </span>
              </button>
            ))}
          </div>
          <div className="kp-pane" {...tabs.panel(2)}>
            <dl className="kp-info">
              <div>
                <dt>{app("filmSource")}</dt>
                <dd>YouTube</dd>
              </div>
              <div>
                <dt>{app("filmLineOriginal")}</dt>
                <dd>English · whisper.cpp</dd>
              </div>
              <div>
                <dt>{app("filmLineTranslation")}</dt>
                <dd>{t("demoTarget") + " · " + t("stepTranslateTool")}</dd>
              </div>
              <div>
                <dt>{app("filmFiles")}</dt>
                <dd>
                  <code>video.srt · video.bilingual.srt</code>
                </dd>
              </div>
            </dl>
          </div>
          <div className="kp-pane" {...tabs.panel(3)}>
            <p className="kp-label">{app("filmStyleSize")}</p>
            <div className="kp-choices kp-three">
              {(
                [
                  ["s", "playerSizeSmall"],
                  ["m", "playerSizeMedium"],
                  ["l", "playerSizeLarge"],
                ] as const
              ).map(([value, key]) => (
                <button key={value} type="button" data-size={value} aria-pressed={size === value} onClick={() => setSize(value)}>
                  {t(key)}
                </button>
              ))}
            </div>
            <p className="kp-label">{app("filmModeHeading")}</p>
            <div className="kp-choices kp-two">
              {modeLabels.map((label, i) => (
                <button key={i} type="button" data-mode={i} aria-pressed={mode === i} onClick={() => setMode(i)}>
                  {label}
                </button>
              ))}
            </div>
            <p className="kp-label">{app("filmStyleOrder")}</p>
            <div className="kp-choices kp-two">
              <button type="button" data-order="translation" aria-pressed={order === "translation"} onClick={() => setOrder("translation")}>
                {app("filmOrderTranslationFirst")}
              </button>
              <button type="button" data-order="original" aria-pressed={order === "original"} onClick={() => setOrder("original")}>
                {app("filmOrderOriginalFirst")}
              </button>
            </div>
          </div>
        </div>
      </aside>
    </div>
  );
}
