import { existsSync, mkdirSync, readdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const projectRoot = path.resolve(__dirname, "..");
const sourceDocsDir = path.resolve(projectRoot, "..", "docs");
const targetDocsDir = path.resolve(projectRoot, "src", "pages", "docs");
const githubRepoBase = "https://github.com/simplyblock/postbrain/blob/main";

if (!existsSync(sourceDocsDir)) {
  throw new Error(`docs source directory not found: ${sourceDocsDir}`);
}

rmSync(targetDocsDir, { recursive: true, force: true });
mkdirSync(targetDocsDir, { recursive: true });

const docsFiles = readdirSync(sourceDocsDir)
  .filter((name) => name.toLowerCase().endsWith(".md"))
  .sort((a, b) => a.localeCompare(b));
const docsFileSet = new Set(docsFiles.map((name) => name.toLowerCase()));

function titleFromMarkdown(file, markdownText) {
  const heading = markdownText.match(/^#\s+(.+)\s*$/m);
  if (heading?.[1]) {
    return heading[1].replace(/"/g, '\\"');
  }
  const stem = file.replace(/\.md$/i, "");
  if (stem.toLowerCase() === "readme") {
    return "Documentation";
  }
  return stem.replace(/[-_]/g, " ").replace(/(^|\s)\w/g, (m) => m.toUpperCase());
}

for (const file of docsFiles) {
  const fileLower = file.toLowerCase();
  const sourcePath = path.join(sourceDocsDir, file);
  const targetFile = fileLower === "readme.md" ? "index.md" : file;
  const targetPath = path.join(targetDocsDir, targetFile);
  const sourceText = readFileSync(sourcePath, "utf8");
  const title = titleFromMarkdown(file, sourceText);

  // Rewrite markdown links for Astro directory-style routes:
  // docs pages are served as /docs/<slug>/, so sibling links from a docs page
  // need to use ../<slug>/.
  const rewritten = sourceText.replace(/\]\(([^)\s]+?\.md)(#[^)]+)?\)/g, (m, href, anchor) => {
    if (/^(?:[a-z]+:|\/|#)/i.test(href)) {
      return m;
    }

    const normalized = path.posix.normalize(href);
    const targetFileName = path.posix.basename(normalized).toLowerCase();
    if (!docsFileSet.has(targetFileName)) {
      let repoPath = "";
      if (normalized.startsWith("../")) {
        repoPath = normalized.replace(/^(\.\.\/)+/, "");
      } else if (normalized.startsWith("./")) {
        repoPath = `docs/${normalized.slice(2)}`;
      } else {
        repoPath = `docs/${normalized}`;
      }
      const cleanRepoPath = path.posix.normalize(repoPath);
      if (cleanRepoPath.startsWith("..")) {
        return m;
      }
      return `](${githubRepoBase}/${cleanRepoPath}${anchor ?? ""})`;
    }

    const targetSlug = targetFileName === "readme.md" ? "" : targetFileName.replace(/\.md$/i, "");
    const rel = fileLower === "readme.md"
      ? (targetSlug === "" ? "./" : `./${targetSlug}/`)
      : (targetSlug === "" ? "../" : `../${targetSlug}/`);

    return `](${rel}${anchor ?? ""})`;
  });

  const frontmatter = [
    "---",
    "layout: ../../layouts/DocsLayout.astro",
    `title: "${title}"`,
    `sourceFile: "${file}"`,
    "---",
    ""
  ].join("\n");

  writeFileSync(targetPath, `${frontmatter}${rewritten}`, "utf8");
}

console.log(`synced ${docsFiles.length} markdown files from docs/`);
