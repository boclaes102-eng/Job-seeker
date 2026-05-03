// Categorise techs to colour-code badges by domain.
// This list mirrors the categories used by the server-side matcher.
const CATEGORIES: Record<string, string[]> = {
  language: [
    'Python', 'TypeScript', 'JavaScript', 'Go', 'PHP', 'C#', 'SQL', 'Bash',
  ],
  frontend: [
    'React', 'Next.js', 'Three.js', 'Vue', 'Angular', 'Tailwind CSS', 'Vite',
    'Chart.js', 'HTML', 'CSS', 'GLSL',
  ],
  backend: [
    'Node.js', 'Fastify', 'FastAPI', 'asyncio', 'WebSockets', 'REST', 'GraphQL',
  ],
  data: [
    'PostgreSQL', 'Redis', 'SQLite', 'MongoDB', 'Kafka', 'BullMQ', 'Drizzle ORM',
    'XGBoost', 'scikit-learn', 'OpenCV', 'pandas', 'Streamlit',
  ],
  security: [
    'Scapy', 'Nmap', 'mitmproxy', 'ldap3', 'YARA', 'MITRE ATT&CK', 'OpenVAS',
    'Nessus', 'Burp Suite', 'Metasploit', 'Wireshark', 'Active Directory',
  ],
  iot: [
    'Raspberry Pi', 'Arduino', 'PCB Design', 'Firmware', 'Node-RED',
  ],
  infra: [
    'Docker', 'Kubernetes', 'GitHub Actions', 'CI/CD', 'Vercel', 'Railway',
    'Prometheus', 'Grafana', 'AWS', 'S3', 'SQS',
  ],
  ai: [
    'Anthropic Claude', 'Groq', 'Supabase', 'LLM', 'Ollama',
  ],
}

const COLORS: Record<string, string> = {
  language: 'bg-purple-100 text-purple-700 border-purple-200',
  frontend: 'bg-sky-100 text-sky-700 border-sky-200',
  backend: 'bg-emerald-100 text-emerald-700 border-emerald-200',
  data: 'bg-amber-100 text-amber-800 border-amber-200',
  security: 'bg-red-100 text-red-700 border-red-200',
  iot: 'bg-orange-100 text-orange-700 border-orange-200',
  infra: 'bg-slate-200 text-slate-700 border-slate-300',
  ai: 'bg-fuchsia-100 text-fuchsia-700 border-fuchsia-200',
  default: 'bg-gray-100 text-gray-700 border-gray-200',
}

function categoryFor(tech: string): string {
  for (const [cat, techs] of Object.entries(CATEGORIES)) {
    if (techs.includes(tech)) return cat
  }
  return 'default'
}

interface Props {
  tech: string[]
  maxShown?: number
}

export function TechBadges({ tech, maxShown = 6 }: Props) {
  if (!tech || tech.length === 0) return null
  const shown = tech.slice(0, maxShown)
  const extra = tech.length - shown.length
  return (
    <div className="flex flex-wrap gap-1.5">
      {shown.map(t => (
        <span
          key={t}
          className={`inline-flex items-center text-[11px] font-medium px-2 py-0.5 rounded-full border ${COLORS[categoryFor(t)]}`}
        >
          {t}
        </span>
      ))}
      {extra > 0 && (
        <span className="inline-flex items-center text-[11px] font-medium px-2 py-0.5 rounded-full bg-gray-100 text-gray-500 border border-gray-200">
          +{extra} more
        </span>
      )}
    </div>
  )
}
