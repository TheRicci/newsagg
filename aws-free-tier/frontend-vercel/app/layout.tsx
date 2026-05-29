import type { Metadata } from "next"
import "./globals.css"

export const metadata: Metadata = {
  title: "newsagg",
  description: "Personal news aggregator",
}

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode
}>) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  )
}

