function scoreColor(score: number) {
  if (score >= 75) return 'text-emerald-600'
  if (score >= 50) return 'text-yellow-500'
  return 'text-red-400'
}

export function ScoreRing({ score }: { score: number }) {
  return (
    <div className={`flex flex-col items-center justify-center w-14 h-14 rounded-full border-2 shrink-0 ${score >= 75 ? 'border-emerald-400' : score >= 50 ? 'border-yellow-400' : 'border-red-300'}`}>
      <span className={`text-lg font-bold leading-none ${scoreColor(score)}`}>{score}</span>
      <span className="text-[9px] text-gray-400 leading-none mt-0.5">/ 100</span>
    </div>
  )
}
