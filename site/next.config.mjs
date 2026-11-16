// Static export for GitHub Pages. The site is served from /<repo>/, so the
// base path comes from the workflow (PAGES_BASE_PATH) and is empty locally.
const basePath = process.env.PAGES_BASE_PATH ?? "";

/** @type {import('next').NextConfig} */
const nextConfig = {
  output: "export",
  basePath,
  trailingSlash: true,
  images: { unoptimized: true },
  env: { NEXT_PUBLIC_BASE_PATH: basePath },
};

export default nextConfig;
