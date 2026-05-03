import { useState } from 'react'
import type { Job, JobStatus } from '../types/job'
import { SourceBadge } from './SourceBadge'
import { ScoreRing } from './ScoreRing'
import { TechBadges } from './TechBadges'

interface Props {
  job: Job
  onStatusChange: (id: string, status: JobStatus) => void
  onAnalyze: (id: string) => void
  analyzing: boolean
}

// daysSince returns whole days between then and now (rounded down).
function daysSince(date: Date): number {
  const ms = Date.now() - date.getTime()
  return Math.max(0, Math.floor(ms / 86_400_000))
}

export function JobCard({ job, onStatusChange, onAnalyze, analyzing }: Props) {
  const [expanded, setExpanded] = useState(false)
  const postedDate = new Date(job.postedAt)
  const posted = postedDate.toLocaleDateString('en-BE', { day: 'numeric', month: 'short' })
  const days = daysSince(postedDate)
  const isStale = days > 7

  return (
    <article
      onClick={() => setExpanded(e => !e)}
      className={`bg-white border rounded-xl p-5 flex gap-4 shadow-sm hover:shadow-md transition-shadow cursor-pointer select-none ${
        isStale ? 'border-amber-200 bg-amber-50/30' : 'border-gray-200'
      }`}
    >
      <ScoreRing score={job.matchScore} />

      <div className="flex-1 min-w-0">
        <div className="flex items-start justify-between gap-2">
          <div className="flex items-center gap-2 flex-wrap">
            <SourceBadge source={job.source} />
            <span className="text-xs text-gray-400">{posted}</span>
            {isStale && (
              <span
                className="text-[10px] font-medium px-2 py-0.5 rounded-full bg-amber-100 text-amber-700"
                title={`Posted ${days} days ago — likely already filled`}
              >
                {days}d old
              </span>
            )}
          </div>
          <span className="text-gray-300 text-xs shrink-0 mt-0.5">{expanded ? '▲' : '▼'}</span>
        </div>

        {/* Title — clicking opens the job, doesn't toggle the card */}
        <a
          href={job.url}
          target="_blank"
          rel="noopener noreferrer"
          onClick={e => e.stopPropagation()}
          className="block mt-1 text-gray-900 font-semibold text-base hover:text-indigo-600 truncate"
        >
          {job.title}
        </a>
        <p className="text-sm text-gray-500 mt-0.5">
          {job.company}
          {job.location ? ` · ${job.location}` : ''}
        </p>

        {/* Matched-tech badges — at-a-glance reason for the score */}
        {job.matchedTech && job.matchedTech.length > 0 && (
          <div className="mt-2">
            <TechBadges tech={job.matchedTech} maxShown={expanded ? 99 : 6} />
          </div>
        )}

        {/* AI reason */}
        {job.matchReason && (
          <p className={`mt-2 text-sm text-gray-600 italic leading-snug ${expanded ? '' : 'line-clamp-2'}`}>
            {job.matchReason}
          </p>
        )}

        {/* Description snippet — only when expanded */}
        {expanded && job.description && (
          <div className="mt-3 pt-3 border-t border-gray-100">
            <p className="text-xs font-medium text-gray-400 mb-1">Job description</p>
            <p className="text-sm text-gray-600 leading-relaxed line-clamp-6 whitespace-pre-line">
              {job.description.replace(/<[^>]+>/g, ' ').trim()}
            </p>
          </div>
        )}

        {/* Actions — clicks don't bubble up to the card toggle */}
        <div className="mt-3 flex gap-2 flex-wrap" onClick={e => e.stopPropagation()}>
          <button
            onClick={() => onStatusChange(job.id, 'pipeline')}
            className="text-xs px-3 py-1 rounded-full font-medium bg-indigo-50 hover:bg-indigo-100 text-indigo-700 transition-colors"
          >
            + Add to pipeline
          </button>
          <button
            onClick={() => onStatusChange(job.id, 'dismissed')}
            className="text-xs px-3 py-1 rounded-full font-medium bg-red-50 hover:bg-red-100 text-red-600 transition-colors"
          >
            ✕ Ignore
          </button>
          <button
            onClick={() => onAnalyze(job.id)}
            disabled={analyzing}
            className="text-xs px-3 py-1 rounded-full font-medium bg-violet-50 hover:bg-violet-100 text-violet-700 disabled:opacity-50 transition-colors"
          >
            {analyzing ? 'Analyzing…' : '✦ Re-score'}
          </button>
        </div>
      </div>
    </article>
  )
}
