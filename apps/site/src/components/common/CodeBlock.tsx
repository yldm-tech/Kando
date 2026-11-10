// CodeBlock is a command the page shows with a button that copies it, each line after a $ prompt and coloured as the design's terminals are (highlight). The button copies the command itself, without the prompts; it stays hidden without script, and where the browser has no clipboard.
import { Fragment, useEffect, useState } from "react";
import { useWords } from "../../i18n/content";
import { highlight } from "../../lib/highlight";

export function CodeBlock({ text }: { text: string }) {
  const { t } = useWords();
  const [canCopy, setCanCopy] = useState(false);
  const [copied, setCopied] = useState(false);
  useEffect(() => {
    setCanCopy(typeof navigator !== "undefined" && navigator.clipboard !== undefined);
  }, []);
  useEffect(() => {
    if (!copied) {
      return;
    }
    const id = window.setTimeout(() => setCopied(false), 1600);
    return () => window.clearTimeout(id);
  }, [copied]);
  return (
    <div className="code">
      <pre>
        <code>
          {text.split("\n").map((line, i) => (
            <Fragment key={i}>
              {i > 0 ? "\n" : null}
              <span className="p" aria-hidden="true">
                ${" "}
              </span>
              {highlight(line).map((token, j) =>
                token.kind === "" ? (
                  token.text
                ) : (
                  <span key={j} className={token.kind}>
                    {token.text}
                  </span>
                ),
              )}
            </Fragment>
          ))}
        </code>
      </pre>
      <button
        className={"copy" + (copied ? " done" : "")}
        type="button"
        hidden={!canCopy}
        onClick={() => void navigator.clipboard.writeText(text).then(() => setCopied(true))}
      >
        {copied ? t("copiedLabel") : t("copyLabel")}
      </button>
    </div>
  );
}
