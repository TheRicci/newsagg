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
    ufology: "border-purple-800 text-purple-300 bg-purple-950",
    neuroscience: "border-sky-800 text-sky-300 bg-sky-950",
    finance: "border-emerald-800 text-emerald-300 bg-emerald-950",
  }

  const activeColors: Record<string, string> = {
    ufology: "bg-purple-700 text-white border-purple-700",
    neuroscience: "bg-sky-700 text-white border-sky-700",
    finance: "bg-emerald-700 text-white border-emerald-700",
  }

  return (
    <div className="mb-6 flex flex-wrap gap-2">
      <button
        onClick={() => onSelect("")}
        className={`rounded border px-4 py-1.5 text-sm transition-colors ${
          selected === "" ? "border-zinc-100 bg-zinc-100 text-zinc-900" : "border-zinc-700 text-zinc-400 hover:border-zinc-500"
        }`}
      >
        All
      </button>
      {topics.map((topic) => (
        <button
          key={topic.topic}
          onClick={() => onSelect(topic.topic)}
          className={`rounded border px-4 py-1.5 text-sm transition-colors ${
            selected === topic.topic
              ? activeColors[topic.topic] ?? "border-zinc-600 bg-zinc-600 text-white"
              : topicColors[topic.topic] ?? "border-zinc-700 text-zinc-400"
          }`}
        >
          {topic.topic} <span className="ml-1 text-xs opacity-60">{topic.count}</span>
        </button>
      ))}
    </div>
  )
}

