"use client"

import { useState } from "react"

type Article = {
  id: string
  topic: string
  source: string
  title: string
  url: string
  summary: string
  published_at: string
  ai_summary?: string
  ai_key_points?: string[]
  ai_context?: string
  ai_tags?: string[]
  ai_confidence?: string
}

export default function ArticleCard({ article }: { article: Article }) {
  const [expanded, setExpanded] = useState(false)

  const date = new Date(article.published_at).toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
    year: "numeric",
  })

  const topicColors: Record<string, string> = {
    ufology: "bg-purple-950 text-purple-200",
    neuroscience: "bg-sky-950 text-sky-200",
    finance: "bg-emerald-950 text-emerald-200",
  }

  const confidenceColors: Record<string, string> = {
    high: "text-emerald-400",
    medium: "text-amber-400",
    low: "text-zinc-500",
  }

  const hasEnrichment = Boolean(article.ai_summary)

  return (
    <article className="overflow-hidden rounded border border-zinc-800 bg-zinc-900">
      <a href={article.url} target="_blank" rel="noopener noreferrer" className="block p-5 transition-colors hover:bg-zinc-800">
        <div className="mb-3 flex items-center gap-2">
          <span className={`rounded px-2 py-0.5 text-xs font-medium ${topicColors[article.topic] ?? "bg-zinc-800 text-zinc-300"}`}>
            {article.topic}
          </span>
          <span className="text-xs text-zinc-500">{article.source}</span>
          <span className="ml-auto text-xs text-zinc-600">{date}</span>
        </div>
        <h2 className="mb-2 text-sm font-semibold leading-snug text-zinc-100">{article.title}</h2>
        {article.summary && <p className="line-clamp-2 text-xs leading-relaxed text-zinc-400">{article.summary}</p>}
      </a>

      {hasEnrichment && (
        <div className="border-t border-zinc-800">
          <button
            onClick={() => setExpanded((value) => !value)}
            className="flex w-full items-center gap-2 px-5 py-2 text-xs text-zinc-500 transition-colors hover:text-zinc-300"
          >
            <span>{expanded ? "Hide" : "Show"} AI enrichment</span>
            {article.ai_confidence && (
              <span className={`ml-auto ${confidenceColors[article.ai_confidence] ?? "text-zinc-500"}`}>
                {article.ai_confidence} confidence
              </span>
            )}
          </button>

          {expanded && (
            <div className="space-y-3 px-5 pb-5">
              {article.ai_summary && <p className="text-xs leading-relaxed text-zinc-300">{article.ai_summary}</p>}
              {article.ai_key_points && article.ai_key_points.length > 0 && (
                <ul className="space-y-1">
                  {article.ai_key_points.map((point) => (
                    <li key={point} className="flex gap-2 text-xs text-zinc-400">
                      <span className="shrink-0 text-zinc-600">-</span>
                      <span>{point}</span>
                    </li>
                  ))}
                </ul>
              )}
              {article.ai_context && (
                <p className="border-l-2 border-zinc-700 pl-3 text-xs leading-relaxed text-zinc-500">
                  {article.ai_context}
                </p>
              )}
              {article.ai_tags && article.ai_tags.length > 0 && (
                <div className="flex flex-wrap gap-1">
                  {article.ai_tags.map((tag) => (
                    <span key={tag} className="rounded bg-zinc-800 px-2 py-0.5 text-xs text-zinc-500">
                      {tag}
                    </span>
                  ))}
                </div>
              )}
            </div>
          )}
        </div>
      )}
    </article>
  )
}

