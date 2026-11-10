// jsonForScript is v as JSON for a <script type="application/json"> block: with <, >, & and the line and paragraph separators escaped, as Go's encoding/json writes them, so that no text in it can end the script element or be read as markup.
export function jsonForScript(v: unknown): string {
  return JSON.stringify(v)
    .replaceAll("<", "\\u003c")
    .replaceAll(">", "\\u003e")
    .replaceAll("&", "\\u0026")
    .replaceAll("\u2028", "\\u2028")
    .replaceAll("\u2029", "\\u2029");
}
