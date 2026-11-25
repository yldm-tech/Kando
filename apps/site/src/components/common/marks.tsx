// The brand marks the page draws beside its words, from AI Logo (github.com/yldm-tech/ai-logo), the collection Kando's own apps share, as buildtime/marks.ts takes the logo strip's: the GitHub mark, Apple's for macOS and the iPhone, Windows', Linux's, Android's and Google Play's. Each is AI Logo's one-color SVG, bundled with the page (?raw) and drawn in the text's color.
import android from "@yldm-tech/ai-logo-static-svg/icons/android.svg?raw";
import apple from "@yldm-tech/ai-logo-static-svg/icons/apple.svg?raw";
import github from "@yldm-tech/ai-logo-static-svg/icons/github.svg?raw";
import googlePlay from "@yldm-tech/ai-logo-static-svg/icons/googleplay.svg?raw";
import linux from "@yldm-tech/ai-logo-static-svg/icons/linux.svg?raw";
import windows from "@yldm-tech/ai-logo-static-svg/icons/microsoft.svg?raw";

type Drawing = { viewBox: string; fillRule?: "evenodd"; inner: string };

// drawing takes from an AI Logo file what the page draws of it: its view box, its fill rule and its shapes, without its title and the style attribute it carries, which the page's Content Security Policy refuses.
function drawing(svg: string): Drawing {
  const open = /^<svg\b[^>]*>/.exec(svg)?.[0] ?? "";
  const viewBox = /\sviewBox="([^"]+)"/.exec(open)?.[1];
  if (!viewBox || !svg.endsWith("</svg>")) {
    throw new Error("not an AI Logo SVG: " + svg.slice(0, 60));
  }
  const inner = svg.slice(open.length, -"</svg>".length).replace(/<title>[^<]*<\/title>/, "");
  return { viewBox, fillRule: /\sfill-rule="evenodd"/.test(open) ? "evenodd" : undefined, inner };
}

function mark(svg: string) {
  const d = drawing(svg.trim());
  return function Mark({ size }: { size: number }) {
    return (
      <svg
        className="mark"
        viewBox={d.viewBox}
        width={size}
        height={size}
        fill="currentColor"
        fillRule={d.fillRule}
        aria-hidden="true"
        dangerouslySetInnerHTML={{ __html: d.inner }}
      />
    );
  };
}

export const GitHubMark = mark(github);
export const AppleMark = mark(apple);
export const WindowsMark = mark(windows);
export const LinuxMark = mark(linux);
export const AndroidMark = mark(android);
export const GooglePlayMark = mark(googlePlay);
