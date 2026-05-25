"use client"

import { useEffect, useState, useCallback } from "react"
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
}

type Topic = {
  topic: string
  count: number
}

export default function Home() {
  const [articles, setArticles] = useState<Article[]>([])
  const [topics, setTopics] = useState<Topic[]>([])
  const [selectedTopic, setSelectedTopic] = useState("")
  const [page, setPage] = useState(1)
  const [loading, setLoading] = useState(false)
  const [hasMore, setHasMore] = useState(true)
  const LIMIT = 20

  // Fetch topics once on mount
  useEffect(() => {
    fetch("/api/topics")
      .then((r) => r.json())
      .then(setTopics)
  }, [])

  // Fetch articles when topic or page changes
  const fetchArticles = useCallback(async (topic: string, p: number, append: boolean) => {
    setLoading(true)
    const params = new URLSearchParams({
      limit: String(LIMIT),
      page: String(p),
      ...(topic ? { topic } : {}),
    })
    const res = await fetch(`/api/articles?${params}`)
    const data = await res.json()
    const fetched: Article[] = data.articles ?? []
    setArticles((prev) => append ? [...prev, ...fetched] : fetched)
    setHasMore(fetched.length === LIMIT)
    setLoading(false)
  }, [])

  useEffect(() => {
    setPage(1)
    fetchArticles(selectedTopic, 1, false)
  }, [selectedTopic, fetchArticles])

  const loadMore = () => {
    const next = page + 1
    setPage(next)
    fetchArticles(selectedTopic, next, true)
  }

  const handleTopicSelect = (topic: string) => {
    setSelectedTopic(topic)
  }

  return (
    <main className="min-h-screen bg-zinc-950 text-zinc-100">
      <div className="max-w-2xl mx-auto px-4 py-10">
        <div className="mb-8">
          <h1 className="text-2xl font-bold text-zinc-100 mb-1">newsagg</h1>
          <p className="text-sm text-zinc-500">ufology · neuroscience · finance</p>
        </div>

        <TopicFilter
          topics={topics}
          selected={selectedTopic}
          onSelect={handleTopicSelect}
        />

        <div className="flex flex-col gap-3">
          {articles.map((article) => (
            <ArticleCard key={article.id} article={article} />
          ))}
        </div>

        {loading && (
          <p className="text-center text-zinc-600 text-sm mt-8">loading...</p>
        )}

        {!loading && hasMore && (
          <button
            onClick={loadMore}
            className="w-full mt-8 py-2.5 rounded-xl border border-zinc-800 text-zinc-400 text-sm hover:border-zinc-600 hover:text-zinc-200 transition-colors"
          >
            load more
          </button>
        )}

        {!loading && !hasMore && articles.length > 0 && (
          <p className="text-center text-zinc-700 text-sm mt-8">no more articles</p>
        )}
      </div>
    </main>
  )
}
