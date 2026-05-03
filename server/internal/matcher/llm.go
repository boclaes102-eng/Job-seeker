package matcher

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"job-seeker/server/internal/models"
)

// Matcher orchestrates the two-stage scoring pipeline:
//  1. Fast deterministic score for every job (DeterministicScore)
//  2. LLM referee for jobs in the uncertainty band
//
// Email drafting also lives here because it shares the Ollama client.
type Matcher struct {
	host  string
	model string
	cache *scoreCache
}

func New() *Matcher {
	host := os.Getenv("OLLAMA_HOST")
	if host == "" {
		host = "http://localhost:11434"
	}
	model := os.Getenv("OLLAMA_MODEL")
	if model == "" {
		// llama3.2:3b is ~2 GB and 3-5x faster than llama3.1:8b on CPU/modest GPU.
		// For structured judgement (score + 1-sentence reason as JSON) the smaller
		// model is more than adequate — and crucially fast enough to be worth
		// using on every borderline job rather than just the top few.
		model = "llama3.2:3b"
	}
	return &Matcher{host: host, model: model, cache: newScoreCache()}
}

// Model returns the configured model name (used for logging).
func (m *Matcher) Model() string { return m.model }

// ResetCache clears the in-memory score cache.
func (m *Matcher) ResetCache() { m.cache.reset() }

// MatchResult is one LLM scoring response.
type MatchResult struct {
	Score  int    `json:"score"`
	Reason string `json:"reason"`
}

// EmailDraft is the structured output of DraftEmail.
type EmailDraft struct {
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

// ScoredJob is the full output of one job's scoring run.
type ScoredJob struct {
	Score       int
	Reason      string
	MatchedTech []string
	UsedLLM     bool
}

// ─── Public scoring API ──────────────────────────────────────────────────────

// ScoreJob runs the two-stage pipeline. Cached on (jobURL, descHash, profileHash).
//
// Always returns a valid ScoredJob — on LLM failure we fall back to the
// deterministic result, never block.
func (m *Matcher) ScoreJob(ctx context.Context, job *models.Job, idx *CandidateIndex) ScoredJob {
	cacheKey := m.cache.key(job.URL, job.Description, idx.Hash)
	if e, ok := m.cache.get(cacheKey); ok {
		return ScoredJob{
			Score:       e.Score,
			Reason:      e.Reason,
			MatchedTech: e.MatchedTech,
			UsedLLM:     false,
		}
	}

	det := DeterministicScore(job, idx)

	// Stage 2: LLM referee, only for borderline jobs (skip if a deal-breaker
	// already capped the score — those are clear no-go).
	if !det.NeedsLLM {
		m.cache.put(cacheKey, det.Total, det.Reason, det.MatchedTech)
		return ScoredJob{Score: det.Total, Reason: det.Reason, MatchedTech: det.MatchedTech, UsedLLM: false}
	}

	finalScore, finalReason, err := m.refineWithLLM(ctx, job, idx, det)
	if err != nil {
		slog.Warn("matcher.llm_failed", "url", job.URL, "err", err)
		m.cache.put(cacheKey, det.Total, det.Reason, det.MatchedTech)
		return ScoredJob{Score: det.Total, Reason: det.Reason, MatchedTech: det.MatchedTech, UsedLLM: false}
	}
	m.cache.put(cacheKey, finalScore, finalReason, det.MatchedTech)
	return ScoredJob{Score: finalScore, Reason: finalReason, MatchedTech: det.MatchedTech, UsedLLM: true}
}

// Score is the legacy single-job entry point used by the AnalyzeJob endpoint
// (the user-clicked "Re-score" button). It always consults the LLM regardless
// of band, because the user is explicitly asking for a fresh judgement.
func (m *Matcher) Score(ctx context.Context, job *models.Job, profile string) (int, string, []string, error) {
	idx := BuildCandidateIndex(profile)
	det := DeterministicScore(job, idx)

	if det.DealBreaker != "" {
		return det.Total, det.Reason, det.MatchedTech, nil
	}
	score, reason, err := m.refineWithLLM(ctx, job, idx, det)
	if err != nil {
		return det.Total, det.Reason, det.MatchedTech, nil
	}
	return score, reason, det.MatchedTech, nil
}

// ─── LLM referee ─────────────────────────────────────────────────────────────

func (m *Matcher) refineWithLLM(ctx context.Context, job *models.Job, idx *CandidateIndex, det ScoreBreakdown) (int, string, error) {
	prompt := buildRefinePrompt(job, idx, det)

	req := ollamaRequest{
		Model:     m.model,
		Messages:  []ollamaMessage{{Role: "user", Content: prompt}},
		Stream:    false,
		Format:    "json",
		KeepAlive: "10m", // keep the model loaded between batched calls
		Options: ollamaOptions{
			Temperature: 0,   // deterministic judgement
			NumPredict:  220, // short JSON response is plenty
			TopP:        0.95,
		},
	}

	var resp ollamaResponse
	if err := m.callOllama(ctx, "/api/chat", req, &resp); err != nil {
		return det.Total, det.Reason, err
	}

	var result MatchResult
	content := strings.TrimSpace(resp.Message.Content)
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return det.Total, det.Reason, fmt.Errorf("llm json parse: %w (content=%q)", err, content)
	}

	// Sanity-clamp: refuse adjustments greater than ±25 points from the
	// deterministic anchor. The LLM is a referee, not a re-scorer.
	final := clampAdjustment(det.Total, result.Score, 25)

	reason := strings.TrimSpace(result.Reason)
	if reason == "" {
		reason = det.Reason
	}
	return final, reason, nil
}

// buildRefinePrompt is intentionally compact and structured. The LLM gets:
// - the candidate summary (~400 tokens)
// - the deterministic findings as bullet points
// - the job title, company, truncated description
// - clear adjustment rules
//
// Total prompt is around 1500 tokens — small enough to run quickly even on
// a 3B model, large enough to capture the relevant context.
func buildRefinePrompt(job *models.Job, idx *CandidateIndex, det ScoreBreakdown) string {
	matched := strings.Join(det.MatchedTech, ", ")
	if matched == "" {
		matched = "(none)"
	}
	missing := strings.Join(det.MissingTech, ", ")
	if missing == "" {
		missing = "(none detected)"
	}
	roleMatch := det.RoleMatch
	if roleMatch == "" {
		roleMatch = "(no role keyword in title)"
	}

	return fmt.Sprintf(`You are a job-fit referee. A deterministic algorithm has already scored a job posting; your job is to slightly adjust the score based on context the algorithm cannot see.

%s

Deterministic baseline: %d / 100
- candidate techs found in this job: %s
- techs the job requires but candidate lacks: %s
- title role match: %s

Job title: %s
Company: %s
Location: %s
Job description (truncated):
%s

Adjustment rules:
- The candidate "Tech" line above is GROUND TRUTH. NEVER claim the candidate lacks a tech listed there.
- You may adjust the baseline by AT MOST ±25 points.
- Do NOT score on location — the user judges distance themselves. Ignore where the job is.
- Adjust DOWN if: the job requires a major tech the candidate clearly lacks (e.g. Java/Salesforce/SAP/Ruby on Rails) and that tech is the primary requirement; the seniority is a hard mismatch (e.g. "10+ years required"); the role is in a wrong domain (e.g. mechanical engineering, sales, accounting).
- Adjust UP if: the job description mentions multiple of the candidate's strengths together (e.g. full-stack + cybersecurity); the role explicitly welcomes medior or junior; the company stack is unusually well-aligned with the candidate's projects.
- Otherwise keep the baseline. Most jobs don't need adjustment.

Return ONLY valid JSON, no other text:
{"score": integer 0-100, "reason": "one specific sentence explaining your decision and naming the primary required tech"}`,
		idx.Summary,
		det.Total,
		matched,
		missing,
		roleMatch,
		job.Title,
		job.Company,
		job.Location,
		truncate(job.Description, 1500),
	)
}

func clampAdjustment(baseline, llmScore, maxDelta int) int {
	if llmScore < 0 {
		llmScore = 0
	}
	if llmScore > 100 {
		llmScore = 100
	}
	delta := llmScore - baseline
	if delta > maxDelta {
		return baseline + maxDelta
	}
	if delta < -maxDelta {
		return baseline - maxDelta
	}
	return llmScore
}

// ─── Email drafting ──────────────────────────────────────────────────────────

func (m *Matcher) DraftEmail(ctx context.Context, job *models.Job, profile string) (EmailDraft, error) {
	idx := BuildCandidateIndex(profile)

	prompt := fmt.Sprintf(`You are writing a professional job application email on behalf of the candidate below.

Rules:
- Write in the SAME language as the job description (Dutch if Dutch, English if English, French if French).
- 3–4 short paragraphs: brief intro, why you fit (cite 1–2 specific things from the candidate's tech that match), 1 specific achievement that's relevant, closing with call to action.
- Be specific — reference the job title and company name explicitly.
- Do NOT use generic filler ("very motivated", "look forward to hearing from you") without substance.
- Use the candidate's actual name: Bo Claes. Never "[Your Name]" or similar placeholders.
- Mention naturally — not boastfully — that this job was discovered via a personal job-scouting tool Bo built (a Go backend that scrapes job boards, scores postings with a local LLM, surfaces best matches).
- Sign off with: Bo Claes | boclaes102@gmail.com | github.com/boclaes102-eng

%s

Job title: %s
Company: %s
Job description:
%s

Return ONLY valid JSON:
{"subject": "short professional subject line", "body": "full email body, plain text, newlines as \\n"}`,
		idx.Summary,
		job.Title, job.Company,
		truncate(job.Description, 2000),
	)

	req := ollamaRequest{
		Model:     m.model,
		Messages:  []ollamaMessage{{Role: "user", Content: prompt}},
		Stream:    false,
		Format:    "json",
		KeepAlive: "10m",
		Options: ollamaOptions{
			Temperature: 0.4,
			NumPredict:  600,
		},
	}

	var resp ollamaResponse
	if err := m.callOllama(ctx, "/api/chat", req, &resp); err != nil {
		return EmailDraft{}, err
	}

	var draft EmailDraft
	if err := json.Unmarshal([]byte(resp.Message.Content), &draft); err != nil {
		return EmailDraft{}, fmt.Errorf("draft parse: %w", err)
	}
	return draft, nil
}

// ─── HTTP plumbing ───────────────────────────────────────────────────────────

type ollamaRequest struct {
	Model     string          `json:"model"`
	Messages  []ollamaMessage `json:"messages"`
	Stream    bool            `json:"stream"`
	Format    string          `json:"format,omitempty"`
	KeepAlive string          `json:"keep_alive,omitempty"`
	Options   ollamaOptions   `json:"options,omitempty"`
}

type ollamaOptions struct {
	Temperature float64 `json:"temperature"`
	NumPredict  int     `json:"num_predict,omitempty"`
	TopP        float64 `json:"top_p,omitempty"`
}

type ollamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaResponse struct {
	Message ollamaMessage `json:"message"`
}

// Shared HTTP client. Per-call deadlines come from the context passed in.
var ollamaHTTP = &http.Client{
	Timeout: 0,
	Transport: &http.Transport{
		MaxIdleConns:       8,
		MaxConnsPerHost:    8,
		IdleConnTimeout:    60 * time.Second,
		DisableCompression: false,
	},
}

func (m *Matcher) callOllama(ctx context.Context, path string, payload any, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.host+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := ollamaHTTP.Do(req)
	if err != nil {
		return fmt.Errorf("ollama: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("ollama status %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("ollama decode: %w", err)
	}
	return nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
