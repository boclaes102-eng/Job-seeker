const colors: Record<string, string> = {
  jobat: 'bg-orange-100 text-orange-800',
  adzuna: 'bg-green-100 text-green-800',
  linkedin: 'bg-sky-100 text-sky-800',
}

export function SourceBadge({ source }: { source: string }) {
  return (
    <span className={`inline-block px-2 py-0.5 rounded text-xs font-semibold uppercase tracking-wide ${colors[source] ?? 'bg-gray-100 text-gray-600'}`}>
      {source}
    </span>
  )
}
