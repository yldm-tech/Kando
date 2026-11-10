// The page draws brand marks from AI Logo (github.com/yldm-tech/ai-logo), the collection Kando's own apps share: the websites of the works-with strip, the AI services of the AI card and Claude Code in the MCP tab. Each mark is written into the built site as an SVG file from @yldm-tech/ai-logo-static-svg, so the page still loads nothing from elsewhere.
import { copyFileSync, existsSync, mkdirSync } from "node:fs";
import { createRequire } from "node:module";
import { dirname, join } from "node:path";
import type { Brand } from "./schema";

// siteMarks maps a featured site's domain to its mark's ID in AI Logo. The map is explicit rather than guessed from the domain, as a domain's name can be another brand's mark there: qq.com is Tencent Video in the catalog, while AI Logo's qq is the messenger. A featured site without a mark here keeps the logo the app's catalog has for it.
export const siteMarks: Readonly<Record<string, string>> = {
  "youtube.com": "youtube",
  "bilibili.com": "bilibili",
  "tiktok.com": "tiktok",
  "twitter.com": "x",
  "instagram.com": "instagram",
  "facebook.com": "facebook",
  "vimeo.com": "vimeo",
  "twitch.tv": "twitch",
  "nicovideo.jp": "niconico",
  "weibo.com": "weibo",
  "xiaohongshu.com": "xiaohongshu",
  "reddit.com": "reddit",
  "dailymotion.com": "dailymotion",
  "soundcloud.com": "soundcloud",
  "bandcamp.com": "bandcamp",
  "kick.com": "kick",
  "rumble.com": "rumble",
  "vk.com": "vk",
  "naver.com": "naver",
};

// aiServices are the AI services of the AI card, among the presets of the app's AI Service settings, in the order the card names them.
export const aiServices: readonly Omit<Brand, "src">[] = [
  { name: "EveryAPI", id: "everyapi" },
  { name: "OpenAI", id: "openai" },
  { name: "DeepSeek", id: "deepseek" },
  { name: "Qwen", id: "qwen" },
  { name: "Kimi", id: "kimi" },
  { name: "Ollama", id: "ollama" },
  { name: "LM Studio", id: "lmstudio" },
];

// lightOnDark are the marks AI Logo draws in color for a dark background, white where the others are dark, which the page's white tiles would hide: they are shown in one color.
export const lightOnDark: ReadonlySet<string> = new Set(["kimi"]);

// claudeCode is the MCP tab's mark: the example adds Kando to Claude Code.
export const claudeCode: Omit<Brand, "src"> = { name: "Claude Code", id: "claudecode" };

// iconsDir is the folder of AI Logo's SVG files. The package has no exports map, so it is found by its package.json.
export function iconsDir(): string {
  const require = createRequire(import.meta.url);
  return join(dirname(require.resolve("@yldm-tech/ai-logo-static-svg/package.json")), "icons");
}

// Marks writes AI Logo's marks into the built site's marks folder, each once.
export class Marks {
  private readonly written = new Map<string, string>();

  constructor(
    private readonly out: string,
    private readonly icons: string = iconsDir(),
  ) {}

  // src writes the mark id into the site, in color where AI Logo draws it in color for a light background and in one color otherwise, and returns the path the page links it by.
  src(id: string): string {
    const done = this.written.get(id);
    if (done) {
      return done;
    }
    const names = lightOnDark.has(id) ? [id + ".svg"] : [id + "-color.svg", id + ".svg"];
    for (const name of names) {
      const file = join(this.icons, name);
      if (!existsSync(file)) {
        continue;
      }
      mkdirSync(join(this.out, "marks"), { recursive: true });
      copyFileSync(file, join(this.out, "marks", name));
      const src = "/marks/" + name;
      this.written.set(id, src);
      return src;
    }
    throw new Error(`AI Logo has no mark ${JSON.stringify(id)}`);
  }

  brands(bs: readonly Omit<Brand, "src">[]): Brand[] {
    return bs.map((b) => ({ ...b, src: this.src(b.id) }));
  }
}
