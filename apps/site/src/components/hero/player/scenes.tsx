// The four scenes of the demo film in the hero's player, after the design, each two of its lines long. id is the id of the scene's gradient, unique in the page, as the player and its clips both draw the scenes.

function Scene0({ id }: { id: string }) {
  return (
    <svg viewBox="0 0 320 180" preserveAspectRatio="xMidYMid slice" aria-hidden="true">
      <defs>
        <linearGradient id={id} x1="0" y1="0" x2="0" y2="1">
          <stop offset="0" stopColor="#9CC3DD" />
          <stop offset="1" stopColor="#ECE4CF" />
        </linearGradient>
      </defs>
      <rect width="320" height="180" fill={`url(#${id})`} />
      <circle cx="252" cy="44" r="15" fill="#FFF4D6" />
      <path d="M0 120 L50 70 L95 105 L150 30 L205 95 L250 60 L320 110 L320 180 L0 180Z" fill="#7D98AE" />
      <path d="M150 30 L136 50 L146 47 L152 56 L160 46 L168 52Z M50 70 L40 82 L50 79 L58 86Z M250 60 L238 74 L250 70 L262 76Z" fill="#F4F7FA" />
      <path d="M0 150 Q70 118 150 136 T320 128 L320 180 L0 180Z" fill="#4F7469" />
      <path d="M151 52 C150 70 156 86 150 104 C146 118 156 128 152 140" fill="none" stroke="#BFE6F0" strokeWidth="2.2" strokeLinecap="round" />
    </svg>
  );
}

function Scene1({ id }: { id: string }) {
  return (
    <svg viewBox="0 0 320 180" preserveAspectRatio="xMidYMid slice" aria-hidden="true">
      <defs>
        <linearGradient id={id} x1="0" y1="0" x2="0" y2="1">
          <stop offset="0" stopColor="#8DB9D2" />
          <stop offset="1" stopColor="#EBDDBB" />
        </linearGradient>
      </defs>
      <rect width="320" height="180" fill={`url(#${id})`} />
      <circle cx="238" cy="58" r="17" fill="#FFF2CC" />
      <path d="M0 110 L60 70 L110 95 L170 55 L230 90 L280 65 L320 85 L320 180 L0 180Z" fill="#6A8EA1" />
      <path d="M0 140 Q80 110 160 130 T320 120 L320 180 L0 180Z" fill="#3E6A5B" />
      <path d="M150 180 C170 160 190 150 180 135 C172 124 200 118 215 112 L222 116 C210 122 188 128 196 140 C206 156 188 168 176 180Z" fill="#A9D2DE" />
      <ellipse cx="168" cy="162" rx="5" ry="3" fill="#8E9AA0" />
      <ellipse cx="200" cy="150" rx="4" ry="2.4" fill="#9AA5AA" />
      <ellipse cx="186" cy="136" rx="3" ry="2" fill="#8E9AA0" />
      <ellipse cx="160" cy="174" rx="6" ry="3" fill="#7F8B91" />
    </svg>
  );
}

function Scene2({ id }: { id: string }) {
  return (
    <svg viewBox="0 0 320 180" preserveAspectRatio="xMidYMid slice" aria-hidden="true">
      <defs>
        <linearGradient id={id} x1="0" y1="0" x2="0" y2="1">
          <stop offset="0" stopColor="#A6CBDD" />
          <stop offset="1" stopColor="#E4DCC4" />
        </linearGradient>
      </defs>
      <rect width="320" height="180" fill={`url(#${id})`} />
      <path d="M0 0 L0 180 L140 180 L60 40 L30 0Z" fill="#5D7C8A" />
      <path d="M320 0 L320 180 L180 180 L262 46 L296 0Z" fill="#577585" />
      <path d="M0 60 L0 180 L146 180 L88 96Z" fill="#46636F" />
      <path d="M320 64 L320 180 L174 180 L232 100Z" fill="#41606C" />
      <path d="M146 180 L160 120 L174 180Z" fill="#3E6A5B" />
      <path d="M156 180 C158 160 162 146 160 128 L162 128 C165 146 162 162 164 180Z" fill="#B6DCE6" />
      <path d="M60 40 L88 96 L66 70Z M262 46 L232 100 L254 72Z" fill="#6E8C99" opacity="0.6" />
    </svg>
  );
}

function Scene3({ id }: { id: string }) {
  return (
    <svg viewBox="0 0 320 180" preserveAspectRatio="xMidYMid slice" aria-hidden="true">
      <defs>
        <linearGradient id={id} x1="0" y1="0" x2="0" y2="1">
          <stop offset="0" stopColor="#F0BE8E" />
          <stop offset="0.55" stopColor="#F6DDB9" />
          <stop offset="1" stopColor="#F6DDB9" />
        </linearGradient>
      </defs>
      <rect width="320" height="180" fill={`url(#${id})`} />
      <circle cx="230" cy="74" r="20" fill="#FFE6B0" />
      <path d="M0 88 L80 80 L160 86 L240 78 L320 84 L320 180 L0 180Z" fill="#9AAE84" />
      <path d="M0 104 Q160 92 320 102 L320 180 L0 180Z" fill="#6F8F5E" />
      <path
        d="M150 96 C100 104 220 112 170 120 C110 130 250 140 160 152 C80 162 230 172 150 180"
        fill="none"
        stroke="#C6E6EC"
        strokeWidth="7"
        strokeLinecap="round"
      />
      <ellipse cx="236" cy="132" rx="14" ry="4" fill="none" stroke="#C6E6EC" strokeWidth="4" opacity="0.85" />
    </svg>
  );
}

const scenes = [Scene0, Scene1, Scene2, Scene3];

export const sceneCount = scenes.length;

// Scene draws scene n with its gradient named id.
export function Scene({ n, id }: { n: number; id: string }) {
  const Drawn = scenes[n];
  if (!Drawn) {
    throw new Error("no scene " + n);
  }
  return <Drawn id={id} />;
}
