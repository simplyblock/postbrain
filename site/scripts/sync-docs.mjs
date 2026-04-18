import { existsSync, mkdirSync, readdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const projectRoot = path.resolve(__dirname, "..");
const sourceDocsDir = path.resolve(projectRoot, "..", "docs");
const targetDocsDir = path.resolve(projectRoot, "src", "pages", "docs");
const generatedDir = path.resolve(projectRoot, "src", "generated");
const githubRepoBase = "https://github.com/simplyblock/postbrain/blob/main";
const repo = process.env.GITHUB_REPOSITORY?.split("/")[1] ?? "";
const usePagesBase = process.env.GITHUB_ACTIONS === "true" && repo !== "";
const docsAssetBasePrefix = usePagesBase ? `/${repo}` : "";

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
  let rewritten = sourceText.replace(/\]\(([^)\s]+?\.md)(#[^)]+)?\)/g, (m, href, anchor) => {
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

  // Rewrite docs image links to site public assets. Keep source markdown
  // GitHub-friendly while generated docs pages reference /assets/... paths.
  rewritten = rewritten.replace(/\]\(\.\.\/site\/public\/assets\//g, `](${docsAssetBasePrefix}/assets/`);
  // Also support already-absolute /assets links.
  rewritten = rewritten.replace(/\]\(\/assets\//g, `](${docsAssetBasePrefix}/assets/`);

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

// Build nav data so DocsLayout can import it statically at build time
// rather than scanning the filesystem at prerender time (which breaks
// because __dirname resolves differently inside the compiled .prerender bundle).

function buildNavSections(files) {
  const fileContents = new Map(
    files.map((f) => [f.toLowerCase(), readFileSync(path.join(sourceDocsDir, f), "utf8")])
  );

  function titleFromSlug(slug) {
    return slug
      .replace(/[-_]/g, " ")
      .split(/\s+/)
      .filter(Boolean)
      .map((word) => word.charAt(0).toUpperCase() + word.slice(1))
      .join(" ");
  }

  function pageForFile(file) {
    const text = fileContents.get(file.toLowerCase()) ?? "";
    const heading = text.match(/^#\s+(.+)\s*$/m)?.[1];
    const fileLower = file.toLowerCase();
    const slug = fileLower === "readme.md" ? "" : file.replace(/\.md$/i, "");
    const title = heading ?? (slug === "" ? "Documentation" : titleFromSlug(slug));
    return { title, slug };
  }

  const readmeText = fileContents.get("readme.md") ?? "";
  const available = new Map(files.map((f) => [f.toLowerCase(), f]));
  const sections = [];
  const seen = new Set();

  if (readmeText) {
    let current = null;
    for (const line of readmeText.split(/\r?\n/)) {
      const headingMatch = line.match(/^##\s+(.+?)\s*$/);
      if (headingMatch) {
        current = { title: headingMatch[1], files: [] };
        sections.push(current);
        continue;
      }
      const linkMatch = line.match(/^\s*-\s+\[[^\]]+\]\(\.\/([^)]+?\.md)\)/);
      if (linkMatch && current) {
        const file = path.basename(linkMatch[1]);
        const lower = file.toLowerCase();
        if (!seen.has(lower) && available.has(lower)) {
          current.files.push(available.get(lower));
          seen.add(lower);
        }
      }
    }
  }

  const navSections = [];
  for (const section of sections) {
    const pages = section.files
      .map((f) => pageForFile(f))
      .filter((p) => p.slug !== "");
    if (pages.length > 0) {
      navSections.push({ title: section.title, pages });
    }
  }

  if (navSections.length === 0) {
    const fallback = files
      .map((f) => pageForFile(f))
      .filter((p) => p.slug !== "");
    navSections.push({ title: "Contents", pages: fallback });
  } else {
    const uncategorized = files
      .filter((f) => !seen.has(f.toLowerCase()) && f.toLowerCase() !== "readme.md")
      .map((f) => pageForFile(f))
      .filter((p) => p.slug !== "");
    if (uncategorized.length > 0) {
      navSections.push({ title: "More", pages: uncategorized });
    }
  }

  return navSections;
}

mkdirSync(generatedDir, { recursive: true });
const navSections = buildNavSections(docsFiles);
writeFileSync(
  path.join(generatedDir, "docs-nav.json"),
  JSON.stringify({ sections: navSections }, null, 2),
  "utf8"
);
