import { useEffect, useState, useCallback, useRef } from 'react'
import type { Job, JobStatus } from './types/job'
import { fetchJobs, updateStatus, analyzeJob, resetJobs } from './api/client'
import { JobCard } from './components/JobCard'
import { FilterBar, type DateRange } from './components/FilterBar'
import { SourceTabs } from './components/SourceTabs'
import { ProfileEditor } from './components/ProfileEditor'
import { PipelineView } from './components/PipelineView'
import { IgnoredView } from './components/IgnoredView'
import { RefreshProgress } from './components/RefreshProgress'

type Tab = 'jobs' | 'pipeline' | 'profile' | 'ignored'
type RefreshPhase = 'scraping' | 'hydrating' | 'scoring' | 'done'

interface Progress {
  phase: RefreshPhase
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


export default function App() {
  const [tab, setTab] = useState<Tab>('jobs')
  const [allJobs, setAllJobs] = useState<Job[]>([])
  const [viewSource, setViewSource] = useState('')
  const [selectedSources, setSelectedSources] = useState(['linkedin', 'adzuna'])
  const [dateRange, setDateRange] = useState<DateRange>('all')
  const [radius, setRadius] = useState(50)
  const [maxJobs, setMaxJobs] = useState(50)
  const [progress, setProgress] = useState<Progress | null>(null)
  const [analyzingId, setAnalyzingId] = useState<string | null>(null)
  const esRef = useRef<EventSource | null>(null)

  const loadJobs = useCallback(async () => {
    const data = await fetchJobs()
    setAllJobs(data)
  }, [])

  useEffect(() => { loadJobs() }, [loadJobs])

  async function handleRefresh() {
    if (esRef.current) esRef.current.close()
    setProgress({ phase: 'scraping', scraped: 0, totalScrapes: 0, hydrated: 0, totalToHydrate: 0, scored: 0, totalToScore: 0, currentJob: '', log: [], errors: [] })

    const params = new URLSearchParams({ sources: selectedSources.join(','), radius: String(radius), dateRange, maxJobs: String(maxJobs) })
    const es = new EventSource(`/api/jobs/refresh/stream?${params}`)
    esRef.current = es

    es.onmessage = (e) => {
      const ev = JSON.parse(e.data)

      // Handle done outside setProgress — calling async loadJobs() inside an updater is not allowed
      if (ev.type === 'done') {
        es.close()
        setProgress(prev => prev ? { ...prev, phase: 'done', currentJob: '' } : prev)
        loadJobs()
        return
      }

      setProgress(prev => {
        if (!prev) return prev
        const log = [...prev.log]
        switch (ev.type) {
          case 'scrape_start':
            return { ...prev, totalScrapes: ev.total }
          case 'source_done':
            log.push(`✓ ${ev.source} / "${ev.query}" — ${ev.count ?? 0} jobs`)
            return { ...prev, scraped: prev.scraped + 1, log }
          case 'source_error':
            log.push(`✗ ${ev.source} / "${ev.query}": ${ev.error}`)
            return { ...prev, scraped: prev.scraped + 1, errors: [...prev.errors, `${ev.source}/${ev.query}: ${ev.error}`], log }
          case 'dedup':
            log.push(`Dedup: ${ev.total ?? 0} raw → ${ev.unique ?? 0} unique`)
            return { ...prev, totalToScore: ev.unique, log }
          case 'hydrate_start':
            log.push(`Fetching ${ev.total ?? 0} LinkedIn descriptions…`)
            return { ...prev, phase: 'hydrating', totalToHydrate: ev.total, log }
          case 'hydrate_progress':
            return { ...prev, hydrated: ev.current }
          case 'scoring':
            log.push(`[${ev.current}/${ev.total}] ${ev.company} — ${ev.title} (${ev.score})`)
            return { ...prev, phase: 'scoring', scored: ev.current, currentJob: `${ev.company} — ${ev.title}` }
          default:
            return prev
        }
      })
    }
    es.onerror = () => {
      setProgress(prev => prev ? { ...prev, errors: [...prev.errors, 'Connection lost'] } : prev)
      es.close()
    }
  }

  async function handleStatusChange(id: string, s: JobStatus) {
    await updateStatus(id, s)
    if (s === 'dismissed' || s === 'pipeline') {
      setAllJobs(prev => prev.filter(j => j.id !== id))
    } else {
      setAllJobs(prev => prev.map(j => j.id === id ? { ...j, status: s } : j))
    }
  }

  async function handleReset() {
    await resetJobs()
    setAllJobs([])
    setProgress(null)
  }

  async function handleAnalyze(id: string) {
    setAnalyzingId(id)
    try {
      const result = await analyzeJob(id)
      setAllJobs(prev => prev.map(j => j.id === id
        ? { ...j, matchScore: result.score, matchReason: result.reason, matchedTech: result.matchedTech }
        : j))
    } finally {
      setAnalyzingId(null)
    }
  }

  // Source tab filters the display; all other filters are scraping-only params
  const bySource = viewSource ? allJobs.filter(j => j.source === viewSource) : allJobs
  const sorted = [...bySource].sort((a, b) => b.matchScore - a.matchScore || new Date(b.fetchedAt).getTime() - new Date(a.fetchedAt).getTime())

  const counts: Record<string, number> = {
    '': allJobs.length,
    adzuna: allJobs.filter(j => j.source === 'adzuna').length,
    linkedin: allJobs.filter(j => j.source === 'linkedin').length,
  }

  const isRefreshing = progress !== null && progress.phase !== 'done'

  return (
    <div className="min-h-screen bg-gray-50">
      <header className="bg-white border-b border-gray-200 px-6 py-4 flex items-center justify-between">
        <div>
          <h1 className="text-xl font-bold text-gray-900">Job Scout</h1>
          <p className="text-xs text-gray-400">Bo Claes · Belgium</p>
        </div>
        <nav className="flex gap-1">
          {(['jobs', 'pipeline', 'ignored', 'profile'] as Tab[]).map(t => (
            <button
              key={t}
              onClick={() => setTab(t)}
              className={`px-4 py-1.5 rounded-lg text-sm font-medium transition-colors ${tab === t ? 'bg-indigo-600 text-white' : 'text-gray-500 hover:bg-gray-100'}`}
            >
              {t === 'jobs' ? `Jobs (${sorted.length})` : t === 'pipeline' ? 'Pipeline' : t === 'ignored' ? 'Ignored' : 'My Profile'}
            </button>
          ))}
        </nav>
      </header>

      <main className="max-w-4xl mx-auto px-4 py-6 flex flex-col gap-4">
        {tab === 'jobs' && (
          <>
            <FilterBar
              dateRange={dateRange}
              radius={radius}
              maxJobs={maxJobs}
              selectedSources={selectedSources}
              hasJobs={allJobs.length > 0}
              loading={isRefreshing}
              onDateChange={setDateRange}
              onRadiusChange={setRadius}
              onMaxJobsChange={setMaxJobs}
              onSourcesChange={setSelectedSources}
              onRefresh={handleRefresh}
              onReset={handleReset}
            />

            <SourceTabs source={viewSource} counts={counts} onChange={setViewSource} />

            {progress && (
              <RefreshProgress
                phase={progress.phase}
                scraped={progress.scraped}
                totalScrapes={progress.totalScrapes}
                hydrated={progress.hydrated}
                totalToHydrate={progress.totalToHydrate}
                scored={progress.scored}
                totalToScore={progress.totalToScore}
                currentJob={progress.currentJob}
                log={progress.log}
                errors={progress.errors}
              />
            )}

            {sorted.length === 0 && !isRefreshing && !progress && (
              <div className="text-center py-16 text-gray-400">
                <p className="text-4xl mb-3">🔍</p>
                <p className="text-sm">No jobs yet. Hit <strong>Refresh jobs</strong> to start scouting.</p>
              </div>
            )}

            <div className="flex flex-col gap-4">
              {sorted.map(job => (
                <JobCard
                  key={job.id}
                  job={job}
                  onStatusChange={handleStatusChange}
                  onAnalyze={handleAnalyze}
                  analyzing={analyzingId === job.id}
                />
              ))}
            </div>
          </>
        )}

        {tab === 'pipeline' && <PipelineView />}
        {tab === 'ignored' && <IgnoredView onRestore={loadJobs} />}
        {tab === 'profile' && <ProfileEditor />}
      </main>
    </div>
  )
}
