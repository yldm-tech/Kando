// The river itself: a few thousand points that flow left to right along a slow, winding band, each a soft amber dot that brightens and fades on its own beat. The points are placed by the vertex shader straight in the canvas's space, so there is no camera to set up and the river fills the canvas at any size. It draws only while the hero is on screen, lowers its resolution when frames come slowly, and gives up, leaving the glows, when they stay slow.
import { PerformanceMonitor } from "@react-three/drei";
import { Canvas, useFrame } from "@react-three/fiber";
import { animate } from "motion";
import { useEffect, useMemo, useRef, useState } from "react";
import { BufferAttribute, BufferGeometry, NormalBlending, ShaderMaterial, type IUniform } from "three";
import { ease } from "../../common/motion";

const vertex = /* glsl */ `
  attribute vec4 seed;
  uniform float time;
  uniform float pixels;
  uniform float aspect;
  varying float vAlpha;
  varying float vHue;
  varying float vBokeh;

  // course is the river's middle at x, which winds slowly over time.
  float course(float x, float t) {
    return 0.16 * sin(x * 1.9 + t * 0.11) + 0.07 * sin(x * 4.3 - t * 0.17 + 1.3) + 0.06;
  }

  void main() {
    float depth = seed.w;
    float speed = 0.018 + 0.05 * seed.y * (0.4 + depth);
    float x = mod(seed.x * 2.6 + time * speed, 2.6) - 1.3;
    // Points spread across the band, thickest in the middle, and the band breathes wider and narrower along its length.
    float across = (seed.z - 0.5) * 2.0;
    across = sign(across) * pow(abs(across), 1.6);
    float width = 0.2 + 0.09 * sin(x * 2.7 + time * 0.07 + seed.y);
    float y = course(x, time) + across * width;
    y += 0.012 * sin(time * (0.6 + seed.y) + seed.x * 40.0);
    gl_Position = vec4(x, y * aspect, 0.0, 1.0);

    float twinkle = 0.55 + 0.45 * sin(time * (0.8 + 1.6 * seed.y) + seed.z * 60.0);
    float edges = smoothstep(1.3, 0.85, abs(x));
    float core = 1.0 - smoothstep(0.0, 1.0, abs(across));
    // Softer behind the headline in the middle, so the words stay easy to read.
    float aside = mix(0.5, 1.0, smoothstep(0.12, 0.55, abs(x)));
    vAlpha = (0.3 + 0.7 * depth) * twinkle * edges * aside * (0.45 + 0.55 * core);
    vHue = fract(seed.y * 3.7 + seed.z);
    // One point in eight is a wide, faint bokeh rather than a spark; together they are the river's glow.
    vBokeh = step(0.875, fract(seed.x * 7.0 + seed.w * 3.0));
    float spark = (2.5 + 9.0 * depth * depth) * (0.8 + 0.4 * twinkle);
    float bokeh = 26.0 + 34.0 * seed.y;
    gl_PointSize = pixels * mix(spark, bokeh, vBokeh);
    vAlpha *= mix(1.0, 0.16 * (0.6 + 0.4 * core), vBokeh);
  }
`;

const fragment = /* glsl */ `
  varying float vAlpha;
  varying float vHue;
  varying float vBokeh;

  void main() {
    float d = length(gl_PointCoord - 0.5);
    // A bright core in a soft halo, so that each point reads as light rather than as a dot.
    float glow = smoothstep(0.5, 0.0, d);
    float spark = 0.35 * glow * glow + 0.65 * smoothstep(0.22, 0.0, d);
    float alpha = vAlpha * mix(spark, glow * glow, vBokeh);
    // Gold to the icon's orange, as in the hero's glows.
    vec3 gold = vec3(1.0, 0.74, 0.32);
    vec3 amber = vec3(0.98, 0.58, 0.18);
    vec3 orange = vec3(0.93, 0.42, 0.10);
    vec3 color = vHue < 0.5 ? mix(gold, amber, vHue * 2.0) : mix(amber, orange, (vHue - 0.5) * 2.0);
    // The canvas composites premultiplied colors.
    gl_FragColor = vec4(color * alpha, alpha);
  }
`;

// Particles is the river's points; their count follows the canvas's width.
function Particles({ count }: { count: number }) {
  const geometry = useMemo(() => {
    const g = new BufferGeometry();
    const seeds = new Float32Array(count * 4);
    for (let i = 0; i < seeds.length; i++) {
      seeds[i] = Math.random();
    }
    g.setAttribute("seed", new BufferAttribute(seeds, 4));
    // Positions are the shader's; three still wants the attribute to know how many points to draw.
    g.setAttribute("position", new BufferAttribute(new Float32Array(count * 3), 3));
    return g;
  }, [count]);
  const uniforms = useMemo<Record<string, IUniform<number>>>(() => ({ time: { value: Math.random() * 100 }, pixels: { value: 1 }, aspect: { value: 1 } }), []);
  const material = useMemo(
    () =>
      new ShaderMaterial({
        vertexShader: vertex,
        fragmentShader: fragment,
        uniforms,
        transparent: true,
        depthWrite: false,
        depthTest: false,
        blending: NormalBlending,
        premultipliedAlpha: true,
      }),
    [uniforms],
  );
  useEffect(() => () => geometry.dispose(), [geometry]);
  useEffect(() => () => material.dispose(), [material]);

  useFrame((state, delta) => {
    const u = material.uniforms;
    if (u["time"] && u["pixels"] && u["aspect"]) {
      u["time"].value += Math.min(delta, 0.1);
      u["pixels"].value = state.viewport.dpr;
      // The band keeps its height in pixels as the canvas widens.
      u["aspect"].value = Math.min(1.6, Math.max(0.7, state.size.width / state.size.height / 1.6));
    }
  });

  return <points geometry={geometry} material={material} frustumCulled={false} />;
}

export default function RiverScene({ onGiveUp }: { onGiveUp: () => void }) {
  const box = useRef<HTMLDivElement>(null);
  const [visible, setVisible] = useState(true);
  const [dpr, setDpr] = useState(() => Math.min(window.devicePixelRatio, 1.75));
  const [count] = useState(() => Math.round(Math.min(4800, Math.max(1200, window.innerWidth * 3))));

  // The river draws only while the hero is on screen.
  useEffect(() => {
    const el = box.current;
    if (!el) {
      return;
    }
    const seen = new IntersectionObserver(([entry]) => setVisible(entry?.isIntersecting ?? false));
    seen.observe(el);
    return () => seen.disconnect();
  }, []);

  const fadeIn = () => {
    if (box.current) {
      void animate(box.current, { opacity: [0, 1] }, { duration: 2.4, ease });
    }
  };

  return (
    <div ref={box} className="river pointer-events-none absolute inset-x-0 top-0 h-[900px] opacity-0 phone:h-[640px]">
      <Canvas
        dpr={dpr}
        frameloop={visible ? "always" : "never"}
        gl={{ alpha: true, antialias: false, powerPreference: "low-power" }}
        onCreated={fadeIn}
        aria-hidden="true"
      >
        <PerformanceMonitor onDecline={() => setDpr(1)} onFallback={onGiveUp} flipflops={3}>
          <Particles count={count} />
        </PerformanceMonitor>
      </Canvas>
    </div>
  );
}
