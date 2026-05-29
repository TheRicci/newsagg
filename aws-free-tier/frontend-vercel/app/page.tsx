"use client"

import { useCallback, useEffect, useState } from "react"
import ArticleCard from "./components/ArticleCard"
import TopicFilter from "./components/TopicFilter"

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

type Topic = {
  topic: string
  count: number
}

const LIMIT = 20

export default function Home() {
  const [articles, setArticles] = useState<Article[]>([])
  const [topics, setTopics] = useState<Topic[]>([])
  const [selectedTopic, setSelectedTopic] = useState("")
  const [page, setPage] = useState(1)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState("")
  const [hasMore, setHasMore] = useState(true)

  useEffect(() => {
    fetch("/api/topics")
      .then((response) => {
        if (!response.ok) {
          throw new Error("Could not load topics")
        }
        return response.json()
      })
      .then(setTopics)
      .catch((err: Error) => setError(err.message))
  }, [])

  const fetchArticles = useCallback(async (topic: string, pageNumber: number, append: boolean) => {
    setLoading(true)
    setError("")

    const params = new URLSearchParams({
      limit: String(LIMIT),
      page: String(pageNumber),
      ...(topic ? { topic } : {}),
    })

    try {
      const response = await fetch(`/api/articles?${params}`)
      if (!response.ok) {
        throw new Error("Could not load articles")
      }
      const data = await response.json()
      const fetched: Article[] = data.articles ?? []
      setArticles((previous) => (append ? [...previous, ...fetched] : fetched))
      setHasMore(fetched.length === LIMIT)
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not load articles")
      setHasMore(false)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    setPage(1)
    fetchArticles(selectedTopic, 1, false)
  }, [selectedTopic, fetchArticles])

  const loadMore = () => {
    const nextPage = page + 1
    setPage(nextPage)
    fetchArticles(selectedTopic, nextPage, true)
  }

  return (
    <main className="min-h-screen bg-zinc-950 text-zinc-100">
      <div className="mx-auto max-w-2xl px-4 py-10">
        <header className="mb-8">
          <h1 className="mb-1 text-2xl font-bold text-zinc-100">newsagg</h1>
          <p className="text-sm text-zinc-500">ufology / neuroscience / finance</p>
        </header>

        <TopicFilter topics={topics} selected={selectedTopic} onSelect={setSelectedTopic} />

        {error && (
          <div className="mb-4 rounded border border-red-900 bg-red-950 px-4 py-3 text-sm text-red-200">
            {error}
          </div>
        )}

        <div className="flex flex-col gap-3">
          {articles.map((article) => (
            <ArticleCard key={article.id} article={article} />
          ))}
        </div>

        {!loading && articles.length === 0 && !error && (
          <p className="mt-8 text-center text-sm text-zinc-600">No articles yet</p>
        )}

        {loading && <p className="mt-8 text-center text-sm text-zinc-600">Loading...</p>}

        {!loading && hasMore && articles.length > 0 && (
          <button
            onClick={loadMore}
            className="mt-8 w-full rounded border border-zinc-800 py-2.5 text-sm text-zinc-400 transition-colors hover:border-zinc-600 hover:text-zinc-200"
          >
            Load more
          </button>
        )}
      </div>
    </main>
  )
}

