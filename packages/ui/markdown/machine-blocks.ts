/**
 * BMAD machine-block preprocessor.
 *
 * Agent comments produced by the BMAD pipeline (dev, dev-fix, code-review,
 * cr-resolve, pr-publish, testarch) embed a structured machine-readable header
 * the sidecar parser uses to compute the next state. The convention is:
 *
 *     <!-- {role}-note -->
 *     ```yaml
 *     role: dev
 *     status: GREEN
 *     branch: multica/tim-1
 *     ...
 *     ```
 *
 *     ## Implementation complete ...   <-- pretty markdown for humans
 *
 * Rendering a wall of YAML as a code block is ugly. This preprocessor finds the
 * pattern and replaces it with structured HTML (a labelled card with a definition
 * list and a nested table for `repos:` arrays). The raw comment body is left
 * untouched on the server, so the sidecar's regex-based parser keeps working —
 * we only rewrite the markdown before it goes into ReactMarkdown.
 */

const MARKER_RE = /<!--\s*(?<kind>[a-z][a-z0-9-]*)\s*-->\s*\n+```ya?ml\s*\n(?<body>[\s\S]*?)\n```/gi;

interface ParsedField {
  key: string;
  value: string;
}

interface ParsedRepo {
  fields: ParsedField[];
}

interface ParsedBlock {
  topLevel: ParsedField[];
  repos: ParsedRepo[];
}

const LIST_KEYS = new Set(["repos", "files_changed", "tests_run"]);

function parseLightYaml(src: string): ParsedBlock {
  // Minimal parser tuned for BMAD machine blocks. Handles:
  //   key: value
  //   key: |\n  multi-line block
  //   listKey:\n  - subkey: val\n    subkey2: val
  const lines = src.split("\n");
  const topLevel: ParsedField[] = [];
  const repos: ParsedRepo[] = [];

  let i = 0;
  while (i < lines.length) {
    const raw = lines[i] ?? "";
    const line = raw.replace(/\s+$/, "");
    if (!line.trim()) { i++; continue; }

    const m = /^([A-Za-z_][\w-]*)\s*:\s*(.*)$/.exec(line);
    if (!m || m[1] === undefined) { i++; continue; }
    const key = m[1];
    const rest = m[2] ?? "";

    // Block scalar: key: |\n  ...
    if (rest === "|" || rest === ">") {
      i++;
      const blockLines: string[] = [];
      while (i < lines.length) {
        const l = lines[i] ?? "";
        if (!l.trim()) { blockLines.push(""); i++; continue; }
        const indent = l.match(/^(\s*)/)?.[1]?.length ?? 0;
        if (indent < 2) break;
        blockLines.push(l.slice(2));
        i++;
      }
      topLevel.push({ key, value: blockLines.join("\n").replace(/\n+$/, "") });
      continue;
    }

    // List key with no inline value: key:\n  - ...
    if (rest === "" && i + 1 < lines.length && /^\s*-\s/.test(lines[i + 1] ?? "")) {
      i++;
      // For `repos:`, parse each `- repo: ...` block with its indented children.
      // For other list keys, collect bullet lines as a single joined value.
      if (key === "repos") {
        while (i < lines.length && /^\s*-\s/.test(lines[i] ?? "")) {
          // Collect this repo entry and its continuation lines
          const repoFields: ParsedField[] = [];
          const firstLine = (lines[i] ?? "").replace(/^\s*-\s*/, "");
          const fm = /^([A-Za-z_][\w-]*)\s*:\s*(.*)$/.exec(firstLine);
          if (fm && fm[1] !== undefined && fm[2] !== undefined) repoFields.push({ key: fm[1], value: fm[2] });
          i++;
          while (i < lines.length) {
            const ll = lines[i] ?? "";
            if (/^\s*-\s/.test(ll)) break;            // next repo
            if (!ll.trim()) { i++; continue; }
            const indent = ll.match(/^(\s*)/)?.[1]?.length ?? 0;
            if (indent < 4) break;                     // dedent out of this repo
            const cm = /^\s+([A-Za-z_][\w-]*)\s*:\s*(.*)$/.exec(ll);
            if (cm && cm[1] !== undefined && cm[2] !== undefined) repoFields.push({ key: cm[1], value: cm[2] });
            i++;
          }
          repos.push({ fields: repoFields });
        }
      } else {
        // Generic list — collect as joined value
        const bullets: string[] = [];
        while (i < lines.length && /^\s*-\s/.test(lines[i] ?? "")) {
          bullets.push((lines[i] ?? "").replace(/^\s*-\s*/, ""));
          i++;
        }
        topLevel.push({ key, value: bullets.join("\n") });
      }
      continue;
    }

    // Plain scalar
    topLevel.push({ key, value: rest });
    i++;
  }

  void LIST_KEYS;
  return { topLevel, repos };
}

function escapeHtml(s: string): string {
  return s
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#39;");
}

function autolink(s: string): string {
  // Very tight autolink: only http/https URLs that look like a single token
  const URL = /^https?:\/\/\S+$/;
  if (URL.test(s.trim())) {
    return `<a href="${escapeHtml(s.trim())}" target="_blank" rel="noopener noreferrer">${escapeHtml(s.trim())}</a>`;
  }
  return escapeHtml(s);
}

function statusBadgeClass(status: string): string {
  const s = status.trim().toLowerCase();
  if (["green", "passed", "created", "updated", "merged", "ready", "ready_for_dev"].includes(s)) return "mb-badge mb-badge-good";
  if (["red", "failed", "blocked", "error", "rejected"].includes(s)) return "mb-badge mb-badge-bad";
  if (["existing", "pending", "in_progress", "running"].includes(s)) return "mb-badge mb-badge-neutral";
  return "mb-badge";
}

function renderField(field: ParsedField): string {
  const label = field.key.replace(/_/g, " ");
  const value = field.value;
  let valueHtml: string;
  if (field.key === "status" || field.key === "pr_status") {
    valueHtml = `<span class="${statusBadgeClass(value)}">${escapeHtml(value)}</span>`;
  } else if (value.includes("\n")) {
    valueHtml = `<pre class="mb-multiline">${escapeHtml(value)}</pre>`;
  } else {
    valueHtml = autolink(value);
  }
  return `<div class="mb-row"><div class="mb-key">${escapeHtml(label)}</div><div class="mb-val">${valueHtml}</div></div>`;
}

function renderRepo(repo: ParsedRepo): string {
  const rows = repo.fields.map(renderField).join("");
  return `<div class="mb-repo">${rows}</div>`;
}

function renderBlock(kind: string, parsed: ParsedBlock): string {
  const role = parsed.topLevel.find((f) => f.key === "role")?.value ?? kind.replace(/-note$|-opened$|-marker$/, "");
  const status = parsed.topLevel.find((f) => f.key === "status")?.value ?? "";

  const headerStatus = status
    ? `<span class="${statusBadgeClass(status)}">${escapeHtml(status)}</span>`
    : "";

  // Skip role and status from the body since they're in the header
  const bodyFields = parsed.topLevel.filter((f) => f.key !== "role" && f.key !== "status");
  const bodyRows = bodyFields.map(renderField).join("");

  const reposHtml = parsed.repos.length
    ? `<div class="mb-section"><div class="mb-section-label">Repos</div>${parsed.repos.map(renderRepo).join("")}</div>`
    : "";

  return [
    `<div class="mb-card" data-kind="${escapeHtml(kind)}">`,
    `<div class="mb-header"><span class="mb-role">${escapeHtml(role)}</span>${headerStatus}</div>`,
    bodyRows ? `<div class="mb-body">${bodyRows}</div>` : "",
    reposHtml,
    `</div>`,
  ].join("");
}

/**
 * Find `<!-- *-{note,opened,marker} -->` markers followed by a ```yaml block
 * and replace the entire match (marker + fence + body) with structured HTML.
 *
 * The replacement is wrapped in blank lines so react-markdown sees it as a
 * standalone block (HTML at the top level is honored by rehype-raw).
 */
export function preprocessMachineBlocks(input: string): string {
  return input.replace(MARKER_RE, (...args: unknown[]) => {
    const match = args[0] as string;
    const groups = args[args.length - 1] as { kind?: string; body?: string } | undefined;
    try {
      const kind = groups?.kind ?? "machine";
      const body = groups?.body ?? "";
      const parsed = parseLightYaml(body);
      const html = renderBlock(kind, parsed);
      return `\n\n${html}\n\n`;
    } catch {
      return match; // on parse error, leave original untouched
    }
  });
}
