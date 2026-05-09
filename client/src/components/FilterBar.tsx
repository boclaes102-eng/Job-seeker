export type DateRange = 'all' | 'today' | '3days' | 'week' | 'month'
export type Seniority = 'all' | 'junior' | 'medior'

const SENIORITY_OPTIONS: { value: Seniority; label: string }[] = [
  { value: 'all', label: 'All levels' },
  { value: 'junior', label: 'Junior' },
  { value: 'medior', label: 'Medior' },
]

const DATE_OPTIONS: { value: DateRange; label: string }[] = [
  { value: 'all', label: 'Any date' },
  { value: 'today', label: 'Today' },
  { value: '3days', label: 'Last 3 days' },
  { value: 'week', label: 'This week' },
  { value: 'month', label: 'This month' },
]

const RADIUS_OPTIONS = [
  { value: 10, label: '10 km' },
  { value: 25, label: '25 km' },
  { value: 50, label: '50 km' },
  { value: 100, label: '100 km' },
  { value: 0, label: 'Any distance' },
]

const MAX_JOBS_OPTIONS = [
  { value: 25, label: '25 jobs' },
  { value: 50, label: '50 jobs' },
  { value: 100, label: '100 jobs' },
  { value: 200, label: '200 jobs' },
  { value: 0, label: 'No limit' },
]

const ALL_SOURCES = ['linkedin', 'adzuna'] as const
const SOURCE_LABELS: Record<string, string> = { linkedin: 'LinkedIn', adzuna: 'Adzuna' }

interface Props {
  dateRange: DateRange
  radius: number
  maxJobs: number
  selectedSources: string[]
  seniority: Seniority
  hasJobs: boolean
  loading: boolean
  onDateChange: (v: DateRange) => void
  onRadiusChange: (v: number) => void
  onMaxJobsChange: (v: number) => void
  onSourcesChange: (v: string[]) => void
  onSeniorityChange: (v: Seniority) => void
  onRefresh: () => void
  onReset: () => void
}

export function FilterBar({
  dateRange, radius, maxJobs, selectedSources, seniority, hasJobs, loading,
  onDateChange, onRadiusChange, onMaxJobsChange, onSourcesChange, onSeniorityChange, onRefresh, onReset,
}: Props) {
  function toggleSource(src: string) {
    if (selectedSources.includes(src)) {
      if (selectedSources.length === 1) return
      onSourcesChange(selectedSources.filter(s => s !== src))
    } else {
      onSourcesChange([...selectedSources, src])
    }
  }

  function handleReset() {
    if (confirm('Clear all jobs except your pipeline and applied ones?')) onReset()
  }

  return (
    <div className="bg-white border border-gray-200 rounded-xl px-5 py-4 shadow-sm w-full flex items-end justify-between gap-4">

      {/* Left: all filters */}
      <div className="flex items-end gap-4 flex-wrap">
        <div className="flex flex-col gap-1.5">
          <label className="text-xs font-medium text-gray-500">Scrape from</label>
          <div className="flex gap-1.5">
            {ALL_SOURCES.map(src => {
              const on = selectedSources.includes(src)
              return (
                <button
                  key={src}
                  onClick={() => toggleSource(src)}
                  className={`px-3 py-1.5 rounded-full text-xs font-medium border transition-colors ${
                    on
                      ? 'bg-indigo-600 border-indigo-600 text-white'
                      : 'bg-white border-gray-300 text-gray-500 hover:border-indigo-400 hover:text-indigo-600'
                  }`}
                >
                  {SOURCE_LABELS[src]}
                </button>
              )
            })}
          </div>
        </div>

        <div className="flex flex-col gap-1.5">
          <label className="text-xs font-medium text-gray-500">Seniority</label>
          <div className="flex gap-1.5">
            {SENIORITY_OPTIONS.map(o => (
              <button
                key={o.value}
                onClick={() => onSeniorityChange(o.value)}
                className={`px-3 py-1.5 rounded-full text-xs font-medium border transition-colors ${
                  seniority === o.value
                    ? 'bg-indigo-600 border-indigo-600 text-white'
                    : 'bg-white border-gray-300 text-gray-500 hover:border-indigo-400 hover:text-indigo-600'
                }`}
              >
                {o.label}
              </button>
            ))}
          </div>
        </div>

        <div className="flex flex-col gap-1">
          <label className="text-xs font-medium text-gray-500">Radius</label>
          <select
            value={radius}
            onChange={e => onRadiusChange(Number(e.target.value))}
            className="border border-gray-300 rounded-lg px-3 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-400"
          >
            {RADIUS_OPTIONS.map(o => <option key={o.value} value={o.value}>{o.label}</option>)}
          </select>
        </div>

        <div className="flex flex-col gap-1">
          <label className="text-xs font-medium text-gray-500">Max jobs</label>
          <select
            value={maxJobs}
            onChange={e => onMaxJobsChange(Number(e.target.value))}
            className="border border-gray-300 rounded-lg px-3 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-400"
          >
            {MAX_JOBS_OPTIONS.map(o => <option key={o.value} value={o.value}>{o.label}</option>)}
          </select>
        </div>

        <div className="flex flex-col gap-1">
          <label className="text-xs font-medium text-gray-500">Posted</label>
          <select
            value={dateRange}
            onChange={e => onDateChange(e.target.value as DateRange)}
            className="border border-gray-300 rounded-lg px-3 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-400"
          >
            {DATE_OPTIONS.map(o => <option key={o.value} value={o.value}>{o.label}</option>)}
          </select>
        </div>
      </div>

      {/* Right: action buttons always anchored to the right */}
      <div className="flex gap-2 shrink-0">
        <button
          onClick={onRefresh}
          disabled={loading}
          className="px-5 py-1.5 rounded-lg bg-indigo-600 hover:bg-indigo-700 disabled:opacity-60 text-white text-sm font-semibold transition-colors whitespace-nowrap"
        >
          {loading ? 'Fetching…' : hasJobs ? '⟳ Refresh jobs' : '🔍 Search jobs'}
        </button>
        {hasJobs && (
          <button
            onClick={handleReset}
            disabled={loading}
            className="px-4 py-1.5 rounded-lg border border-red-200 hover:bg-red-50 disabled:opacity-40 text-red-500 text-sm font-medium transition-colors whitespace-nowrap"
          >
            ✕ Reset
          </button>
        )}
      </div>
    </div>
  )
}
