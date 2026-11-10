// CodeBlock is a command the page shows with a button that copies it. The button stays hidden without script, and where the browser has no clipboard.
import { useEffect, useState } from "react";
import { useWords } from "../../i18n/content";

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
        <code>{text}</code>
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
