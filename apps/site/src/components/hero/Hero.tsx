// The hero: the headline, a row of downloads, one for each system Kando runs on (the disk image for a Mac with Apple silicon, the download panel's Windows and Linux tabs, the phone app's TestFlight beta and Google Play page), the one of the reader's own device the primary, and the demo player in a window.
import site from "virtual:kando/site";
import { useEffect, useState } from "react";
import { Headline, useWords } from "../../i18n/content";
import { googlePlayURL, macDisk, releaseFileURL, testFlightURL } from "../../lib/site";
import { AndroidMark, AppleMark, LinuxMark, WindowsMark } from "../common/marks";
import { Player } from "./player/Player";
import { River } from "./river/River";

// Device is the system whose download the hero makes its primary button: a Mac's until the page runs, and then the reader's own phone's when they read it on an iPhone or an Android phone.
type Device = "mac" | "iphone" | "android";

function deviceOf(userAgent: string): Device {
  if (/iPhone|iPod/.test(userAgent)) {
    return "iphone";
  }
  return /Android/.test(userAgent) ? "android" : "mac";
}

export function Hero() {
  const { t, tf } = useWords();
  const [device, setDevice] = useState<Device>("mac");
  useEffect(() => setDevice(deviceOf(navigator.userAgent)), []);
  const kind = (d: Device) => (d === device ? "btn primary big" : "btn ghost big");
  const shine = (d: Device) => (d === device ? <span className="shine" aria-hidden="true" /> : null);
  // The computers' downloads and the phones', the reader's own first: on a phone its two lead the grid.
  const computers = () => [
    <a key="mac" className={kind("mac")} href={releaseFileURL(site.version, macDisk(site.version, "arm64"))}>
      <AppleMark size={18} />
      {t("heroDownload")}
      {shine("mac")}
    </a>,
    <a key="pc" className="btn ghost big" href="#start" data-os="linux">
      <WindowsMark size={15} />
      <span className="hero-tux">
        <LinuxMark size={17} />
      </span>
      {t("heroOtherSystems")}
    </a>,
  ];
  const phones = () => [
    <a key="iphone" className={kind("iphone")} href={testFlightURL}>
      <AppleMark size={18} />
      {t("heroIphone")}
      {shine("iphone")}
    </a>,
    <a key="android" className={kind("android")} href={googlePlayURL}>
      <AndroidMark size={18} />
      {t("heroAndroid")}
      {shine("android")}
    </a>,
  ];
  return (
    <section className="hero" aria-labelledby="hero-title">
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
        <div className="actions hero-actions" data-enter="">
          {(device === "mac" ? [computers, phones] : [phones, computers]).map((group) => group())}
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
