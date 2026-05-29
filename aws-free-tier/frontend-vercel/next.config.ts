import type { NextConfig } from "next"

const apiGatewayUrl = process.env.API_GATEWAY_URL

const nextConfig: NextConfig = {
  async rewrites() {
    if (!apiGatewayUrl) {
      return []
    }

    return [
      {
        source: "/api/:path*",
        destination: `${apiGatewayUrl.replace(/\/$/, "")}/:path*`,
      },
    ]
  },
}

export default nextConfig

