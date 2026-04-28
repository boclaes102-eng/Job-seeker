interface Props {
  source: string
  counts: Record<string, number>
  onChange: (v: string) => void
}

const tabs = [
  { value: '', label: 'All' },
  { value: 'linkedin', label: 'LinkedIn' },
  { value: 'adzuna', label: 'Adzuna' },
]

export function SourceTabs({ source, counts, onChange }: Props) {
  return (
    <div className="flex gap-2">
      {tabs.map(t => {
        const count = counts[t.value] ?? 0
        const active = source === t.value
        return (
          <button
            key={t.value}
            onClick={() => onChange(t.value)}
            className={`px-4 py-1.5 rounded-full text-sm font-medium transition-colors flex items-center gap-1.5 ${
              active
                ? 'bg-indigo-600 text-white'
                : 'bg-white border border-gray-200 text-gray-500 hover:border-indigo-300 hover:text-indigo-600'
            }`}
          >
            {t.label}
            <span className={`text-xs ${active ? 'text-indigo-200' : 'text-gray-400'}`}>
              {count}
            </span>
          </button>
        )
      })}
    </div>
  )
}
