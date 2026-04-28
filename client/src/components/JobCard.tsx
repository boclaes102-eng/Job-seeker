import { useState } from 'react'
import type { Job, JobStatus } from '../types/job'
import { SourceBadge } from './SourceBadge'
import { ScoreRing } from './ScoreRing'

interface Props {
  job: Job
  onStatusChange: (id: string, status: JobStatus) => void
  onAnalyze: (id: string) => void
  analyzing: boolean
}

export function JobCard({ job, onStatusChange, onAnalyze, analyzing }: Props) {
  const [expanded, setExpanded] = useState(false)
  const posted = new Date(job.postedAt).toLocaleDateString('en-BE', { day: 'numeric', month: 'short' })

  return (
    <article
      onClick={() => setExpanded(e => !e)}
      className="bg-white border border-gray-200 rounded-xl p-5 flex gap-4 shadow-sm hover:shadow-md transition-shadow cursor-pointer select-none"
    >
      <ScoreRing score={job.matchScore} />

      <div className="flex-1 min-w-0">
        <div className="flex items-start justify-between gap-2">
          <div className="flex items-center gap-2 flex-wrap">
            <SourceBadge source={job.source} />
            <span className="text-xs text-gray-400">{posted}</span>
          </div>
          <span className="text-gray-300 text-xs shrink-0 mt-0.5">{expanded ? '▲' : '▼'}</span>
        </div>

        {/* Title — click navigates, doesn't toggle card */}
        <a
          href={job.url}
          target="_blank"
          rel="noopener noreferrer"
          onClick={e => e.stopPropagation()}
          className="block mt-1 text-gray-900 font-semibold text-base hover:text-indigo-600 truncate"
        >
          {job.title}
        </a>
        <p className="text-sm text-gray-500 mt-0.5">{job.company}{job.location ? ` · ${job.location}` : ''}</p>

        {/* AI reason — collapsed: 2 lines, expanded: full */}
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

        {/* Actions — clicks don't bubble up to card toggle */}
        <div
          className="mt-3 flex gap-2 flex-wrap"
          onClick={e => e.stopPropagation()}
        >
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
