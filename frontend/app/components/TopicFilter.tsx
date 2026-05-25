type Topic = {
  topic: string
  count: number
}

type Props = {
  topics: Topic[]
  selected: string
  onSelect: (topic: string) => void
}

export default function TopicFilter({ topics, selected, onSelect }: Props) {
  const topicColors: Record<string, string> = {
    ufology: "border-purple-700 text-purple-300 bg-purple-950",
    neuroscience: "border-blue-700 text-blue-300 bg-blue-950",
    finance: "border-green-700 text-green-300 bg-green-950",
  }

  const activeColors: Record<string, string> = {
    ufology: "bg-purple-700 text-white border-purple-700",
    neuroscience: "bg-blue-700 text-white border-blue-700",
    finance: "bg-green-700 text-white border-green-700",
  }

  return (
    <div className="flex flex-wrap gap-2 mb-6">
      <button
        onClick={() => onSelect("")}
        className={`px-4 py-1.5 rounded-full text-sm border transition-colors ${
          selected === ""
            ? "bg-zinc-100 text-zinc-900 border-zinc-100"
            : "border-zinc-700 text-zinc-400 hover:border-zinc-500"
        }`}
      >
        All
      </button>
      {topics.map((t) => (
        <button
          key={t.topic}
          onClick={() => onSelect(t.topic)}
          className={`px-4 py-1.5 rounded-full text-sm border transition-colors ${
            selected === t.topic
              ? (activeColors[t.topic] ?? "bg-zinc-600 text-white border-zinc-600")
              : (topicColors[t.topic] ?? "border-zinc-700 text-zinc-400")
          }`}
        >
          {t.topic} <span className="opacity-60 text-xs ml-1">{t.count}</span>
        </button>
      ))}
    </div>
  )
}
