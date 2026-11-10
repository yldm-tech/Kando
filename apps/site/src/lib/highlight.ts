// highlight colours a shell command line as the design's terminals do: the command, its subcommand, flags, links and paths, placeholders and operators, or the whole line as a comment. It only splits the line into tokens; the stylesheet gives each kind its colour, and the tokens put back together are the line.

// Kind is a token's colour class: k the command, s its subcommand, f a flag, u a link or path, h a placeholder, o an operator, c a comment, "" plain text and spaces.
export type Kind = "k" | "s" | "f" | "u" | "h" | "o" | "c" | "";

export interface Token {
  text: string;
  kind: Kind;
}

const operators = new Set(["|", "&&", "||", "--", ";"]);

export function highlight(line: string): Token[] {
  if (/^\s*#/.test(line)) {
    return [{ text: line, kind: "c" }];
  }
  const tokens: Token[] = [];
  // word counts the words since the line or the last operator started a command: the first is the command, the second its subcommand.
  let word = 0;
  for (const part of line.split(/(\s+)/)) {
    if (part === "") {
      continue;
    }
    if (/^\s+$/.test(part)) {
      tokens.push({ text: part, kind: "" });
      continue;
    }
    let kind: Kind;
    if (operators.has(part)) {
      kind = "o";
      word = -1;
    } else if (word === 0) {
      kind = "k";
    } else if (part.startsWith("-")) {
      kind = "f";
    } else if (/^<.*>$/.test(part)) {
      kind = "h";
    } else if (/^(https?:|\/|~\/|\.\/)/.test(part) || part.includes("/")) {
      kind = "u";
    } else if (word === 1) {
      kind = "s";
    } else {
      kind = "";
    }
    tokens.push({ text: part, kind });
    word++;
  }
  return tokens;
}
