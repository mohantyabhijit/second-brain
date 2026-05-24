import type { NextConfig } from "next";
import path from "node:path";

const basePath = process.env.NEXT_PUBLIC_BASE_PATH ?? "";
const staticExport = process.env.NEXT_OUTPUT === "export";

const nextConfig: NextConfig = {
  devIndicators: false,
  turbopack: {
    root: path.join(__dirname, "..")
  },
  allowedDevOrigins: ["127.0.0.1"],
  ...(basePath ? { basePath, assetPrefix: basePath } : {}),
  ...(staticExport
    ? {
        output: "export" as const,
        trailingSlash: true,
        images: {
          unoptimized: true
        }
      }
    : {})
};

export default nextConfig;
