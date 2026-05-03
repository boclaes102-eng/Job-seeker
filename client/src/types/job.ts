export type JobStatus = 'new' | 'pipeline' | 'applied' | 'dismissed'

export interface Job {
  id: string
  title: string
  company: string
  location: string
  description: string
  url: string
  source: 'linkedin' | 'adzuna'
  postedAt: string
  fetchedAt: string
  matchScore: number
  matchReason: string
  status: JobStatus
}
