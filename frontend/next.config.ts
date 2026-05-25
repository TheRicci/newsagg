import type { NextConfig } from "next"

const nextConfig: NextConfig = {
  output: "standalone",
  async rewrites() {
    return [
      {
        source: "/api/:path*",
        destination: `${process.env.API_GATEWAY_URL ?? "http://api-gateway"}/:path*`,
      },
    ]
  },
}

export default nextConfig
