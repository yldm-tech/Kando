// The hero: the headline, the Download button, and the demo player in a window.
import { useRef } from "react";
import site from "virtual:kando/site";
import { Headline, useWords } from "../../i18n/content";
import { latestURL } from "../../lib/site";
import { AppleMark } from "../common/marks";
import { useEntrance } from "../common/motion";
import { Player } from "./player/Player";
import { River } from "./river/River";

export function Hero() {
  const { t, tf } = useWords();
  const root = useRef<HTMLElement>(null);
  useEntrance(root);
  return (
    <section className="hero" aria-labelledby="hero-title" ref={root}>
      <div className="hero-sky" aria-hidden="true">
        <span className="blob blob-a" />
        <span className="blob blob-b" />
        <span className="blob blob-c" />
        <River />
        <svg className="grain">
          <filter id="kp-grain">
            <feTurbulence type="fractalNoise" baseFrequency="0.8" numOctaves={2} stitchTiles="stitch" />
            <feColorMatrix values="0 0 0 0 0.6 0 0 0 0 0.5 0 0 0 0 0.4 0 0 0 0.08 0" />
          </filter>
          <rect width="100%" height="100%" filter="url(#kp-grain)" />
        </svg>
      </div>
      <div className="wrap hero-in">
        <div data-enter="">
          <img className="hero-icon" src="/logo.png" width="128" height="128" alt="" />
        </div>
        <h1 id="hero-title" data-enter="">
          <Headline title={t("heroTitle")} mark={t("heroTitleMark")} />
        </h1>
        <p className="lead" data-enter="">
          {t("heroLead")}
        </p>
        <div className="actions" data-enter="">
          <a className="btn primary big" href={latestURL}>
            <AppleMark size={18} />
            {t("heroDownload")}
            <span className="shine" aria-hidden="true" />
          </a>
          <a className="btn ghost big" href="#start" data-os="linux">
            {t("heroOtherSystems")}
          </a>
        </div>
        <p className="meta" data-enter="">
          {tf("heroMeta", { version: site.version })}
        </p>
        <figure className="stage" data-enter="stage">
          <div className="stage-glow" aria-hidden="true" />
          <div className="win dark">
            <div className="win-bar" aria-hidden="true">
              <span className="lights">
                <i />
                <i />
                <i />
              </span>
            </div>
            <Player />
          </div>
          <div className="badge badge-start" aria-hidden="true">
            <span>{t("heroBadgeTranscribe")}</span>
            <strong>whisper.cpp</strong>
          </div>
          <div className="badge badge-end" aria-hidden="true">
            <span className="tick">
              <svg viewBox="0 0 24 24" width="14" height="14">
                <path d="m5 12.5 4.5 4.5L19 7.5" />
              </svg>
            </span>
            <span className="badge-text">
              <span>{t("heroBadgeExport")}</span>
              <code>video.bilingual.srt</code>
            </span>
          </div>
          <figcaption className="sr">{t("shotPlayerAlt")}</figcaption>
        </figure>
      </div>
    </section>
  );
}
