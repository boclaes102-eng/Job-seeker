interface Props {
  phase: 'scraping' | 'hydrating' | 'scoring' | 'done'
  scraped: number
  totalScrapes: number
  hydrated: number
  totalToHydrate: number
  scored: number
  totalToScore: number
  currentJob: string
  log: string[]
  errors: string[]
}

function Bar({ value, max, color }: { value: number; max: number; color: string }) {
  const pct = max === 0 ? 0 : Math.round((value / max) * 100)
  return (
    <div className="flex items-center gap-2">
      <div className="flex-1 h-2 bg-gray-100 rounded-full overflow-hidden">
        <div
          className={`h-full rounded-full transition-all duration-300 ${color}`}
          style={{ width: `${pct}%` }}
        />
      </div>
      <span className="text-xs tabular-nums text-gray-500 w-12 text-right">{value}/{max}</span>
    </div>
  )
}

export function RefreshProgress({ phase, scraped, totalScrapes, hydrated, totalToHydrate, scored, totalToScore, currentJob, log, errors }: Props) {
  // Phase ordering helps decide colour/state for each row.
  const phaseRank: Record<Props['phase'], number> = { scraping: 0, hydrating: 1, scoring: 2, done: 3 }
  const at = phaseRank[phase]

  return (
    <div className="bg-white border border-gray-200 rounded-xl p-4 shadow-sm flex flex-col gap-3 text-sm">

      {/* Scraping phase */}
      <div>
        <div className="flex justify-between mb-1">
          <span className={`font-medium ${at === 0 ? 'text-indigo-600' : 'text-gray-400'}`}>
            {at === 0 ? '⟳ Scraping sources…' : '✓ Sources scraped'}
          </span>
          {at > 0 && <span className="text-gray-400 text-xs">{scraped}/{totalScrapes}</span>}
        </div>
        <Bar value={scraped} max={totalScrapes} color="bg-indigo-400" />
      </div>

      {/* Hydrating phase — only shown when there's something to hydrate */}
      {totalToHydrate > 0 && (
        <div>
          <div className="flex justify-between mb-1">
            <span className={`font-medium ${at === 1 ? 'text-sky-600' : at > 1 ? 'text-gray-400' : 'text-gray-300'}`}>
              {at < 1 ? '↓ Fetch LinkedIn descriptions' : at === 1 ? '↓ Fetching LinkedIn descriptions…' : '✓ Descriptions fetched'}
            </span>
            {at >= 1 && <span className="text-gray-400 text-xs">{hydrated}/{totalToHydrate}</span>}
          </div>
          <Bar value={hydrated} max={totalToHydrate} color="bg-sky-400" />
        </div>
      )}

      {/* Scoring phase */}
      <div>
        <div className="flex justify-between mb-1">
          <span className={`font-medium ${at === 2 ? 'text-violet-600' : at === 3 ? 'text-gray-400' : 'text-gray-300'}`}>
            {at === 3 ? '✓ Jobs scored' : at === 2 ? '✦ Scoring with Ollama…' : '✦ Scoring with Ollama'}
          </span>
          {totalToScore > 0 && <span className="text-gray-400 text-xs">{scored}/{totalToScore}</span>}
        </div>
        <Bar value={scored} max={totalToScore} color="bg-violet-400" />
        {at === 2 && currentJob && (
          <p className="text-xs text-gray-400 mt-1 truncate">→ {currentJob}</p>
        )}
      </div>

      {/* Done */}
      {at === 3 && (
        <p className="text-emerald-600 font-medium">
          ✓ Done — {scored} jobs scored and stored
        </p>
      )}

      {/* Error log */}
      {errors.length > 0 && (
        <details className="text-xs text-red-500">
          <summary className="cursor-pointer">{errors.length} source error{errors.length > 1 ? 's' : ''}</summary>
          <ul className="mt-1 space-y-0.5 pl-2">
            {errors.map((e, i) => <li key={i}>{e}</li>)}
          </ul>
        </details>
      )}

      {/* Recent log */}
      {log.length > 0 && (
        <details className="text-xs text-gray-400">
          <summary className="cursor-pointer">Log ({log.length} events)</summary>
          <ul className="mt-1 space-y-0.5 pl-2 max-h-32 overflow-y-auto">
            {[...log].reverse().map((l, i) => <li key={i}>{l}</li>)}
          </ul>
        </details>
      )}
    </div>
  )
}
