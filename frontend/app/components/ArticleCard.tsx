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
  ai_enriched_at?: string
}

export default function ArticleCard({ article }: { article: Article }) {
  const [expanded, setExpanded] = useState(false)

  const date = new Date(article.published_at).toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
    year: "numeric",
  })

  const topicColors: Record<string, string> = {
    ufology: "bg-purple-900 text-purple-200",
    neuroscience: "bg-blue-900 text-blue-200",
    finance: "bg-green-900 text-green-200",
  }

  const confidenceColors: Record<string, string> = {
    high: "text-green-400",
    medium: "text-yellow-400",
    low: "text-zinc-500",
  }

  const hasEnrichment = !!article.ai_summary

  return (
    <div className="rounded-xl border border-zinc-800 bg-zinc-900 overflow-hidden">
      <a
        href={article.url}
        target="_blank"
        rel="noopener noreferrer"
        className="block p-5 hover:bg-zinc-800 transition-colors"
      >
        <div className="flex items-center gap-2 mb-3">
          <span className={`text-xs px-2 py-0.5 rounded-full font-medium ${topicColors[article.topic] ?? "bg-zinc-700 text-zinc-300"}`}>
            {article.topic}
          </span>
          <span className="text-xs text-zinc-500">{article.source}</span>
          <span className="text-xs text-zinc-600 ml-auto">{date}</span>
        </div>
        <h2 className="text-sm font-semibold text-zinc-100 mb-2 leading-snug">
          {article.title}
        </h2>
        {article.summary && (
          <p className="text-xs text-zinc-400 line-clamp-2 leading-relaxed">
            {article.summary}
          </p>
        )}
      </a>

      {hasEnrichment && (
        <div className="border-t border-zinc-800">
          <button
            onClick={() => setExpanded(!expanded)}
            className="w-full px-5 py-2 flex items-center gap-2 text-xs text-zinc-500 hover:text-zinc-300 transition-colors"
          >
            <span>{expanded ? "▲" : "▼"} AI enrichment</span>
            {article.ai_confidence && (
              <span className={`ml-auto ${confidenceColors[article.ai_confidence] ?? "text-zinc-500"}`}>
                {article.ai_confidence} confidence
              </span>
            )}
          </button>

          {expanded && (
            <div className="px-5 pb-5 space-y-3">
              {article.ai_summary && (
                <p className="text-xs text-zinc-300 leading-relaxed">
                  {article.ai_summary}
                </p>
              )}
              {article.ai_key_points && article.ai_key_points.length > 0 && (
                <ul className="space-y-1">
                  {article.ai_key_points.map((point, i) => (
                    <li key={i} className="text-xs text-zinc-400 flex gap-2">
                      <span className="text-zinc-600 shrink-0">•</span>
                      {point}
                    </li>
                  ))}
                </ul>
              )}
              {article.ai_context && (
                <p className="text-xs text-zinc-500 leading-relaxed border-l-2 border-zinc-700 pl-3">
                  {article.ai_context}
                </p>
              )}
              {article.ai_tags && article.ai_tags.length > 0 && (
                <div className="flex flex-wrap gap-1">
                  {article.ai_tags.map((tag) => (
                    <span key={tag} className="text-xs px-2 py-0.5 rounded-full bg-zinc-800 text-zinc-500">
                      {tag}
                    </span>
                  ))}
                </div>
              )}
            </div>
          )}
        </div>
      )}
    </div>
  )
}
