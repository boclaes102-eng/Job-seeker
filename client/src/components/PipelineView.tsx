import { useEffect, useState } from 'react'
import type { Job, JobStatus } from '../types/job'
import { fetchPipeline, updateStatus, draftEmail } from '../api/client'
import { SourceBadge } from './SourceBadge'
import { ScoreRing } from './ScoreRing'

function openGmailCompose(subject: string, body: string) {
  const url = `https://mail.google.com/mail/?view=cm&fs=1&su=${encodeURIComponent(subject)}&body=${encodeURIComponent(body)}`
  window.open(url, '_blank')
}

interface CardProps {
  job: Job
  onStatusChange: (id: string, status: JobStatus) => void
}

function PipelineCard({ job, onStatusChange }: CardProps) {
  const [drafting, setDrafting] = useState(false)
  const [draftError, setDraftError] = useState('')
  const posted = new Date(job.postedAt).toLocaleDateString('en-BE', { day: 'numeric', month: 'short' })

  async function handleEmail() {
    setDrafting(true)
    setDraftError('')
    try {
      const draft = await draftEmail(job.id)
      openGmailCompose(draft.subject, draft.body)
    } catch {
      setDraftError('Could not generate email. Try again.')
    } finally {
      setDrafting(false)
    }
  }

  return (
    <article className="bg-white border border-gray-200 rounded-xl p-5 flex gap-4 shadow-sm hover:shadow-md transition-shadow">
      <ScoreRing score={job.matchScore} />

      <div className="flex-1 min-w-0">
        <div className="flex items-start gap-2 flex-wrap">
          <SourceBadge source={job.source} />
          <span className="text-xs text-gray-400">{posted}</span>
          {job.status === 'applied' && (
            <span className="text-xs font-medium px-2 py-0.5 rounded-full bg-emerald-100 text-emerald-700">Applied</span>
          )}
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

        <div className="mt-3 flex gap-2 flex-wrap items-center">
          <button
            onClick={handleEmail}
            disabled={drafting}
            className="text-xs px-3 py-1 rounded-full font-medium bg-sky-50 hover:bg-sky-100 text-sky-700 disabled:opacity-50 transition-colors"
          >
            {drafting ? '✦ Writing email…' : '✉ Email via Gmail'}
          </button>
          <button
            onClick={() => onStatusChange(job.id, 'applied')}
            className={`text-xs px-3 py-1 rounded-full font-medium transition-colors ${job.status === 'applied' ? 'bg-emerald-100 text-emerald-700 ring-1 ring-emerald-400' : 'bg-emerald-50 hover:bg-emerald-100 text-emerald-700'}`}
          >
            ✓ Mark applied
          </button>
          <button
            onClick={() => onStatusChange(job.id, 'dismissed')}
            className="text-xs px-3 py-1 rounded-full font-medium bg-red-50 hover:bg-red-100 text-red-600 transition-colors"
          >
            ✕ Remove
          </button>
          {draftError && <span className="text-xs text-red-500">{draftError}</span>}
        </div>
      </div>
    </article>
  )
}

export function PipelineView() {
  const [jobs, setJobs] = useState<Job[]>([])
  const [error, setError] = useState('')

  useEffect(() => {
    fetchPipeline()
      .then(setJobs)
      .catch(e => setError(String(e)))
  }, [])

  async function handleStatusChange(id: string, status: JobStatus) {
    await updateStatus(id, status)
    if (status === 'dismissed') {
      setJobs(prev => prev.filter(j => j.id !== id))
    } else {
      setJobs(prev => prev.map(j => j.id === id ? { ...j, status } : j))
    }
  }

  if (error) return <p className="text-sm text-red-500">{error}</p>

  if (jobs.length === 0) {
    return (
      <div className="text-center py-16 text-gray-400">
        <p className="text-4xl mb-3">📋</p>
        <p className="text-sm">Your pipeline is empty. Add jobs from the Jobs tab.</p>
      </div>
    )
  }

  const pending = jobs.filter(j => j.status !== 'applied')
  const applied = jobs.filter(j => j.status === 'applied')

  return (
    <div className="flex flex-col gap-6">
      {pending.length > 0 && (
        <section>
          <h2 className="text-sm font-semibold text-gray-500 uppercase tracking-wide mb-3">To apply · {pending.length}</h2>
          <div className="flex flex-col gap-4">
            {pending.map(job => <PipelineCard key={job.id} job={job} onStatusChange={handleStatusChange} />)}
          </div>
        </section>
      )}
      {applied.length > 0 && (
        <section>
          <h2 className="text-sm font-semibold text-gray-500 uppercase tracking-wide mb-3">Applied · {applied.length}</h2>
          <div className="flex flex-col gap-4">
            {applied.map(job => <PipelineCard key={job.id} job={job} onStatusChange={handleStatusChange} />)}
          </div>
        </section>
      )}
    </div>
  )
}
