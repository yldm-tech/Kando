// The player's control icons, as the app's player draws them.

export function BackIcon() {
  return (
    <svg viewBox="0 0 12 12" width="12" height="12">
      <path d="M7.5 2 3.5 6l4 4" />
    </svg>
  );
}

export function PanelIcon() {
  return (
    <svg viewBox="0 0 18 18" width="18" height="18">
      <rect x="2" y="3" width="14" height="12" rx="2.5" />
      <path d="M11 3v12" />
    </svg>
  );
}

export function BigPlayIcon() {
  return (
    <svg viewBox="0 0 12 13" width="24" height="26">
      <path d="M2 1.2l9.5 5.3L2 11.8z" />
    </svg>
  );
}

export function SkipBackIcon() {
  return (
    <svg viewBox="0 0 18 18" width="18" height="18">
      <path d="M4 9a5 5 0 1 0 1.5-3.6M4 3v3h3" />
    </svg>
  );
}

export function SkipForwardIcon() {
  return (
    <svg viewBox="0 0 18 18" width="18" height="18">
      <path d="M14 9a5 5 0 1 1-1.5-3.6M14 3v3h-3" />
    </svg>
  );
}

export function PauseIcon() {
  return (
    <svg className="kp-pause-icon" viewBox="0 0 14 16" width="14" height="16">
      <rect x="2" y="1" width="3.6" height="14" rx="1" />
      <rect x="8.4" y="1" width="3.6" height="14" rx="1" />
    </svg>
  );
}

export function PlayIcon() {
  return (
    <svg className="kp-play-icon" viewBox="0 0 12 13" width="14" height="16">
      <path d="M2 1.2l9.5 5.3L2 11.8z" />
    </svg>
  );
}

export function SoundIcon() {
  return (
    <svg viewBox="0 0 18 18" width="18" height="18">
      <path className="kp-cone" d="M3 7v4h3l4 3V4L6 7z" />
      <path className="kp-wave" d="M12.5 6.5a3.5 3.5 0 0 1 0 5" />
      <path className="kp-wave kp-wave-2" d="M14.5 4.5a6.5 6.5 0 0 1 0 9" />
      <path className="kp-cross" d="M12.5 7l4 4M16.5 7l-4 4" />
    </svg>
  );
}

export function LoopIcon() {
  return (
    <svg viewBox="0 0 18 18" width="18" height="18">
      <path d="M3 8V7a3 3 0 0 1 3-3h8l-2-2M15 10v1a3 3 0 0 1-3 3H4l2 2" />
    </svg>
  );
}

export function ModeIcon() {
  return (
    <svg viewBox="0 0 16 14" width="16" height="14">
      <rect x="1" y="1" width="14" height="12" rx="2.5" />
      <path d="M4 6h8M4 9h5" />
    </svg>
  );
}

export function FullScreenIcon() {
  return (
    <svg viewBox="0 0 16 16" width="16" height="16">
      <path d="M2 6V2h4M10 2h4v4M14 10v4h-4M6 14H2v-4" />
    </svg>
  );
}
