import { useEffect, useState } from 'react'
import { fetchProfile, saveProfile } from '../api/client'

// ── Section parsing ──────────────────────────────────────────────────────────

interface Section {
  heading: string
  content: string
}

interface SearchConfig {
  location: string
  city: string
  queries: string[]
}

function parseSections(md: string): Section[] {
  const sections: Section[] = []
  let current: Section | null = null
  for (const line of md.split('\n')) {
    if (line.startsWith('## ')) {
      if (current) sections.push(current)
      current = { heading: line.slice(3).trim(), content: '' }
    } else if (current) {
      current.content += line + '\n'
    }
  }
  if (current) sections.push(current)
  return sections
}

function parseSearch(content: string): SearchConfig {
  const cfg: SearchConfig = { location: 'België', city: '', queries: [] }
  for (const line of content.split('\n')) {
    const t = line.trim()
    if (t.toLowerCase().startsWith('location:')) cfg.location = t.slice(9).trim()
    else if (t.toLowerCase().startsWith('city:')) cfg.city = t.slice(5).trim()
    else if (t.startsWith('- ')) cfg.queries.push(t.slice(2).trim())
  }
  return cfg
}

function buildSearch(cfg: SearchConfig): string {
  let out = `location: ${cfg.location}\ncity: ${cfg.city}\nqueries:\n`
  for (const q of cfg.queries) out += `- ${q}\n`
  return out
}

function rebuildMarkdown(sections: Section[]): string {
  return sections.map(s => `## ${s.heading}\n${s.content}`).join('\n').trimEnd() + '\n'
}

// ── Section card ─────────────────────────────────────────────────────────────

const HINTS: Record<string, string> = {
  'About me': 'A short bio — who you are, what you build, what makes you different.',
  'Experience': 'Past roles, what you built, your impact.',
  'Education': 'Degrees, bootcamps, certificates.',
  'Projects': 'Your key projects with stack and what you did. The more detail, the better the AI matching.',
  'Tech stack': 'Languages, frameworks, tools — organised however you like.',
  'What I\'m looking for': 'Role type, seniority, location, team culture. Ollama reads this to decide fit.',
  'Search': 'Controls what gets scraped. Edit location, city, and search queries below.',
}

interface SectionCardProps {
  section: Section
  onChange: (content: string) => void
}

function SectionCard({ section, onChange }: SectionCardProps) {
  const hint = HINTS[section.heading] ?? ''
  const rows = Math.max(4, section.content.split('\n').length + 1)

  return (
    <div className="bg-white border border-gray-200 rounded-xl p-5 shadow-sm flex flex-col gap-2">
      <div>
        <h3 className="text-sm font-semibold text-gray-800">{section.heading}</h3>
        {hint && <p className="text-xs text-gray-400 mt-0.5">{hint}</p>}
      </div>
      <textarea
        value={section.content}
        onChange={e => onChange(e.target.value)}
        rows={rows}
        spellCheck={false}
        className="w-full font-mono text-sm border border-gray-200 rounded-lg p-3 focus:outline-none focus:ring-2 focus:ring-indigo-400 resize-y bg-gray-50"
      />
    </div>
  )
}

// ── Search section (structured) ───────────────────────────────────────────────

interface SearchCardProps {
  cfg: SearchConfig
  onChange: (cfg: SearchConfig) => void
}

function SearchCard({ cfg, onChange }: SearchCardProps) {
  const [newQuery, setNewQuery] = useState('')

  function addQuery() {
    const q = newQuery.trim()
    if (q && !cfg.queries.includes(q)) {
      onChange({ ...cfg, queries: [...cfg.queries, q] })
    }
    setNewQuery('')
  }

  function removeQuery(q: string) {
    onChange({ ...cfg, queries: cfg.queries.filter(x => x !== q) })
  }

  return (
    <div className="bg-white border border-gray-200 rounded-xl p-5 shadow-sm flex flex-col gap-4">
      <div>
        <h3 className="text-sm font-semibold text-gray-800">Search</h3>
        <p className="text-xs text-gray-400 mt-0.5">Controls what gets scraped from LinkedIn, VDAB and Indeed.</p>
      </div>

      <div className="flex gap-4 flex-wrap">
        <div className="flex flex-col gap-1">
          <label className="text-xs font-medium text-gray-500">Country / region</label>
          <input
            value={cfg.location}
            onChange={e => onChange({ ...cfg, location: e.target.value })}
            className="border border-gray-300 rounded-lg px-3 py-1.5 text-sm w-40 focus:outline-none focus:ring-2 focus:ring-indigo-400"
          />
        </div>
        <div className="flex flex-col gap-1">
          <label className="text-xs font-medium text-gray-500">City (VDAB radius)</label>
          <input
            value={cfg.city}
            onChange={e => onChange({ ...cfg, city: e.target.value })}
            className="border border-gray-300 rounded-lg px-3 py-1.5 text-sm w-40 focus:outline-none focus:ring-2 focus:ring-indigo-400"
          />
        </div>
      </div>

      <div className="flex flex-col gap-2">
        <label className="text-xs font-medium text-gray-500">Search queries</label>
        <div className="flex flex-wrap gap-2">
          {cfg.queries.map(q => (
            <span key={q} className="flex items-center gap-1.5 bg-indigo-50 text-indigo-700 text-xs px-3 py-1 rounded-full">
              {q}
              <button onClick={() => removeQuery(q)} className="text-indigo-400 hover:text-indigo-700 leading-none">×</button>
            </span>
          ))}
        </div>
        <div className="flex gap-2">
          <input
            value={newQuery}
            onChange={e => setNewQuery(e.target.value)}
            onKeyDown={e => e.key === 'Enter' && addQuery()}
            placeholder="e.g. security engineer"
            className="border border-gray-300 rounded-lg px-3 py-1.5 text-sm flex-1 focus:outline-none focus:ring-2 focus:ring-indigo-400"
          />
          <button
            onClick={addQuery}
            className="px-4 py-1.5 rounded-lg bg-indigo-600 hover:bg-indigo-700 text-white text-sm font-medium transition-colors"
          >
            + Add
          </button>
        </div>
      </div>
    </div>
  )
}

// ── Main component ────────────────────────────────────────────────────────────

export function ProfileEditor() {
  const [sections, setSections] = useState<Section[]>([])
  const [search, setSearch] = useState<SearchConfig>({ location: 'België', city: '', queries: [] })
  const [saving, setSaving] = useState(false)
  const [msg, setMsg] = useState('')

  useEffect(() => {
    fetchProfile().then(md => {
      const parsed = parseSections(md)
      const searchIdx = parsed.findIndex(s => s.heading.toLowerCase() === 'search')
      if (searchIdx !== -1) {
        setSearch(parseSearch(parsed[searchIdx].content))
        setSections(parsed.filter((_, i) => i !== searchIdx))
      } else {
        setSections(parsed)
      }
    })
  }, [])

  function updateSection(heading: string, content: string) {
    setSections(prev => prev.map(s => s.heading === heading ? { ...s, content } : s))
  }

  async function handleSave() {
    setSaving(true)
    setMsg('')
    try {
      const searchSection: Section = { heading: 'Search', content: buildSearch(search) }
      const all = [...sections, searchSection]
      await saveProfile(rebuildMarkdown(all))
      setMsg('Saved.')
    } catch {
      setMsg('Save failed.')
    } finally {
      setSaving(false)
    }
  }

  const sectionOrder = ['About me', 'Experience', 'Education', 'Projects', 'Tech stack', 'What I\'m looking for']
  const ordered = [
    ...sectionOrder.map(h => sections.find(s => s.heading === h)).filter(Boolean) as Section[],
    ...sections.filter(s => !sectionOrder.includes(s.heading)),
  ]

  return (
    <div className="flex flex-col gap-4">
      {ordered.map(s => (
        <SectionCard key={s.heading} section={s} onChange={c => updateSection(s.heading, c)} />
      ))}

      <SearchCard cfg={search} onChange={setSearch} />

      <div className="flex items-center gap-3">
        <button
          onClick={handleSave}
          disabled={saving}
          className="px-5 py-2 rounded-lg bg-indigo-600 hover:bg-indigo-700 disabled:opacity-60 text-white text-sm font-semibold transition-colors"
        >
          {saving ? 'Saving…' : 'Save profile'}
        </button>
        {msg && <span className="text-sm text-emerald-600">{msg}</span>}
      </div>
    </div>
  )
}
