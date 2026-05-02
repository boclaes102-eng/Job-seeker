import { useEffect, useState } from 'react'
import type { Job } from '../types/job'
import { fetchIgnored, updateStatus } from '../api/client'
import { SourceBadge } from './SourceBadge'
import { ScoreRing } from './ScoreRing'

interface Props {
  onRestore: () => void
}

export function IgnoredView({ onRestore }: Props) {
  const [jobs, setJobs] = useState<Job[]>([])
  const [error, setError] = useState('')

  useEffect(() => {
    fetchIgnored().then(setJobs).catch(e => setError(String(e)))
  }, [])

  async function handleRestore(id: string) {
    await updateStatus(id, 'new')
    setJobs(prev => prev.filter(j => j.id !== id))
    onRestore()
  }

  if (error) return <p className="text-sm text-red-500">{error}</p>

  if (jobs.length === 0) {
    return (
      <div className="text-center py-16 text-gray-400">
        <p className="text-4xl mb-3">🚫</p>
        <p className="text-sm">No ignored jobs. Jobs you dismiss will appear here.</p>
      </div>
    )
  }

  return (
    <div className="flex flex-col gap-4">
      <p className="text-sm text-gray-400">{jobs.length} ignored job{jobs.length !== 1 ? 's' : ''} — restore any to move it back to the Jobs tab.</p>
      {jobs.map(job => {
        const posted = new Date(job.postedAt).toLocaleDateString('en-BE', { day: 'numeric', month: 'short' })
        return (
          <article key={job.id} className="bg-white border border-gray-200 rounded-xl p-5 flex gap-4 shadow-sm opacity-60 hover:opacity-90 transition-opacity">
            <ScoreRing score={job.matchScore} />
            <div className="flex-1 min-w-0">
              <div className="flex items-start gap-2 flex-wrap">
                <SourceBadge source={job.source} />
                <span className="text-xs text-gray-400">{posted}</span>
              </div>
              <a
                href={job.url}
                target="_blank"
                rel="noopener noreferrer"
                className="block mt-1 text-gray-900 font-semibold text-base hover:text-indigo-600 truncate"
              >
                {job.title}
              </a>
              <p className="text-sm text-gray-500 mt-0.5">{job.company}{job.location ? ` · ${job.location}` : ''}</p>
              {job.matchReason && (
                <p className="mt-2 text-sm text-gray-600 italic leading-snug line-clamp-2">{job.matchReason}</p>
              )}
              <div className="mt-3">
                <button
                  onClick={() => handleRestore(job.id)}
                  className="text-xs px-3 py-1 rounded-full font-medium bg-indigo-50 hover:bg-indigo-100 text-indigo-700 transition-colors"
                >
                  ↩ Restore
                </button>
              </div>
            </div>
          </article>
        )
      })}
    </div>
  )
}
