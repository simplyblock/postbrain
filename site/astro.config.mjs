import { defineConfig } from "astro/config";

const repo = process.env.GITHUB_REPOSITORY?.split("/")[1] ?? "";
const usePagesBase = process.env.GITHUB_ACTIONS === "true" && repo !== "";

export default defineConfig({
  output: "static",
  site: "https://simplyblock.github.io",
  base: usePagesBase ? `/${repo}/` : "/"
});
