const COLLAPSE_CHAR_THRESHOLD = 500;
const COLLAPSE_LINE_THRESHOLD = 15;

export function wrapCollapsible(text: string): string {
  const lines = text.split("\n");
  if (text.length <= COLLAPSE_CHAR_THRESHOLD && lines.length <= COLLAPSE_LINE_THRESHOLD) {
    return text;
  }
  const lineCount = lines.length;
  return `<details><summary>Show more (${lineCount} lines)</summary>\n\n${text}\n\n</details>`;
}
