import type { Job, JobStatus } from '../types/job'

const BASE = '/api'

export async function fetchJobs(source?: string, status?: string): Promise<Job[]> {
  const params = new URLSearchParams()
  if (source) params.set('source', source)
  if (status) params.set('status', status)
  const res = await fetch(`${BASE}/jobs?${params}`)
  if (!res.ok) throw new Error('Failed to fetch jobs')
  return res.json()
}

export async function fetchPipeline(): Promise<Job[]> {
  return fetchJobs(undefined, 'pipeline')
}

export async function fetchIgnored(): Promise<Job[]> {
  return fetchJobs(undefined, 'dismissed')
}


export async function updateStatus(id: string, status: JobStatus): Promise<void> {
  const res = await fetch(`${BASE}/jobs/${id}/status`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ status }),
  })
  if (!res.ok) throw new Error('Status update failed')
}

export async function analyzeJob(id: string): Promise<{ score: number; reason: string }> {
  const res = await fetch(`${BASE}/jobs/${id}/analyze`, { method: 'POST' })
  if (!res.ok) throw new Error('Analysis failed')
  return res.json()
}

export async function draftEmail(id: string): Promise<{ subject: string; body: string }> {
  const res = await fetch(`${BASE}/jobs/${id}/draft-email`, { method: 'POST' })
  if (!res.ok) throw new Error('Email draft failed')
  return res.json()
}

export async function resetJobs(): Promise<{ deleted: number }> {
  const res = await fetch(`${BASE}/jobs`, { method: 'DELETE' })
  if (!res.ok) throw new Error('Reset failed')
  return res.json()
}

export async function clearNewJobs(): Promise<void> {
  await fetch(`${BASE}/jobs/new`, { method: 'DELETE' })
}

export async function fetchProfile(): Promise<string> {
  const res = await fetch(`${BASE}/profile`)
  if (!res.ok) return ''
  const data = await res.json()
  return data.content ?? ''
}

export async function saveProfile(content: string): Promise<void> {
  const res = await fetch(`${BASE}/profile`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ content }),
  })
  if (!res.ok) throw new Error('Save failed')
}
