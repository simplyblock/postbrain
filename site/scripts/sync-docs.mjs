import { cpSync, existsSync, mkdirSync, readdirSync, rmSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const projectRoot = path.resolve(__dirname, "..");
const sourceDocsDir = path.resolve(projectRoot, "..", "docs");
const targetDocsDir = path.resolve(projectRoot, "src", "pages", "docs");

if (!existsSync(sourceDocsDir)) {
  throw new Error(`docs source directory not found: ${sourceDocsDir}`);
}

rmSync(targetDocsDir, { recursive: true, force: true });
mkdirSync(targetDocsDir, { recursive: true });

const docsFiles = readdirSync(sourceDocsDir)
  .filter((name) => name.toLowerCase().endsWith(".md"))
  .sort((a, b) => a.localeCompare(b));

for (const file of docsFiles) {
  cpSync(path.join(sourceDocsDir, file), path.join(targetDocsDir, file));
}

console.log(`synced ${docsFiles.length} markdown files from docs/`);
