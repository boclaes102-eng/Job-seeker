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
//  1. Fast deterministic score for every job (DeterministicScore).
//  2. LLM referee for jobs in the uncertainty band.
//
// Email drafting is also handled here because it shares the Ollama client.
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
		// llama3.2:3b is ~2GB and 3-5x faster than llama3.1:8b on CPU/modest GPU.
		// For scoring (a structured JSON judgement task) the smaller model is
		// more than adequate. Override via OLLAMA_MODEL if you prefer something else.
		model = "llama3.2:3b"
	}
	return &Matcher{host: host, model: model, cache: newScoreCache()}
}

// Model returns the configured model name (used for logging).
func (m *Matcher) Model() string { return m.model }

// ResetCache clears the in-memory score cache.
func (m *Matcher) ResetCache() { m.cache.reset() }

// MatchResult is what one scoring call produces.
type MatchResult struct {
	Score  int    `json:"score"`
	Reason string `json:"reason"`
}

// EmailDraft is the structured output of DraftEmail.
type EmailDraft struct {
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

// ----- Public scoring API -----

// ScoreJob is the new entry point used by the SSE handler. It runs the
// deterministic scorer first; if the result is in the uncertainty band, it
// asks the LLM to refine. Cached on (jobURL, descHash, profileHash).
//
// Returns (score, reason, usedLLM, error). usedLLM is informational so callers
// can report stats — score is always usable even if usedLLM is false.
func (m *Matcher) ScoreJob(ctx context.Context, job *models.Job, idx *CandidateIndex) (int, string, bool, error) {
	// Cache hit?
	cacheKey := m.cache.key(job.URL, job.Description, idx.Hash)
	if e, ok := m.cache.get(cacheKey); ok {
		return e.Score, e.Reason, false, nil
	}

	// Stage 1: deterministic.
	det := DeterministicScore(job, idx)

	// Stage 2: LLM refine, only in the uncertainty band and only if the
	// deal-breaker hasn't already capped the score.
	if !det.NeedsLLM || det.DealBreaker != "" {
		m.cache.put(cacheKey, det.Total, det.Reason)
		return det.Total, det.Reason, false, nil
	}

	finalScore, finalReason, err := m.refineWithLLM(ctx, job, idx, det)
	if err != nil {
		// On LLM failure, fall back to the deterministic score — never block.
		slog.Warn("matcher.llm_failed", "url", job.URL, "err", err)
		m.cache.put(cacheKey, det.Total, det.Reason)
		return det.Total, det.Reason, false, nil
	}
	m.cache.put(cacheKey, finalScore, finalReason)
	return finalScore, finalReason, true, nil
}

// Score is the legacy single-job scoring used by the AnalyzeJob endpoint
// (the "Re-score" button). It always consults the LLM because the user is
// explicitly asking for a fresh judgement.
func (m *Matcher) Score(ctx context.Context, job *models.Job, profile string) (int, string, error) {
	idx := BuildCandidateIndex(profile)
	det := DeterministicScore(job, idx)

	// Force LLM consultation regardless of band — the user clicked re-score.
	if det.DealBreaker != "" {
		// But still respect deal-breakers.
		return det.Total, det.Reason, nil
	}
	score, reason, err := m.refineWithLLM(ctx, job, idx, det)
	if err != nil {
		return det.Total, det.Reason, nil // fall back, don't surface error
	}
	return score, reason, nil
}

// ----- LLM refinement -----

// refineWithLLM calls Ollama with a tight prompt: candidate summary,
// deterministic findings, and the job. The model returns adjusted score + reason.
func (m *Matcher) refineWithLLM(ctx context.Context, job *models.Job, idx *CandidateIndex, det ScoreBreakdown) (int, string, error) {
	prompt := buildRefinePrompt(job, idx, det)

	req := ollamaRequest{
		Model:    m.model,
		Messages: []ollamaMessage{{Role: "user", Content: prompt}},
		Stream:   false,
		Format:   "json",
		Options: ollamaOptions{
			Temperature: 0.1,
			NumPredict:  220, // cap output: short JSON is plenty
		},
	}

	var resp ollamaResponse
	if err := m.callOllama(ctx, "/api/chat", req, &resp); err != nil {
		return det.Total, det.Reason, err
	}

	var result MatchResult
	if err := json.Unmarshal([]byte(resp.Message.Content), &result); err != nil {
		// Malformed JSON: fall back to deterministic score.
		return det.Total, det.Reason, fmt.Errorf("llm json parse: %w", err)
	}

	// Sanity-clamp the LLM score: refuse adjustments greater than ±25 from the
	// deterministic anchor. The LLM is a referee, not a re-scorer.
	final := clampAdjustment(det.Total, result.Score, 25)

	reason := strings.TrimSpace(result.Reason)
	if reason == "" {
		reason = det.Reason
	}
	return final, reason, nil
}

func buildRefinePrompt(job *models.Job, idx *CandidateIndex, det ScoreBreakdown) string {
	matched := strings.Join(det.MatchedTech, ", ")
	if matched == "" {
		matched = "(none)"
	}
	roleMatch := det.RoleMatch
	if roleMatch == "" {
		roleMatch = "(none)"
	}

	return fmt.Sprintf(`You are a job-fit referee. A deterministic algorithm has already produced a baseline score; your job is to adjust it slightly based on context the algorithm cannot see.

%s

Deterministic baseline:
- score: %d / 100
- matched candidate techs in this job: %s
- title role match: %s
- location: %s

Job title: %s
Company: %s
Job description (truncated):
%s

Adjustment rules — read carefully:
- The "Tech" line in the candidate summary is GROUND TRUTH. NEVER claim the candidate lacks a tech listed there.
- You may adjust the baseline by AT MOST ±25 points.
- Adjust DOWN if: the job requires a major tech the candidate clearly lacks (e.g. job requires Java/Salesforce/SAP and candidate has none of these); the seniority is a hard mismatch (e.g. "10+ years required"); the role is in a wrong domain (e.g. mechanical engineering, sales).
- Adjust UP if: the job description mentions multiple of the candidate's strengths together (e.g. full-stack + cybersecurity overlap); the role explicitly welcomes medior or junior; the company stack is unusually well-aligned.
- Otherwise keep the baseline. Most jobs do not need adjustment.

Return ONLY valid JSON:
{"score": integer 0-100, "reason": "one sentence explaining your decision, mentioning the primary required tech and whether the candidate has it"}`,
		idx.Summary,
		det.Total,
		matched,
		roleMatch,
		det.LocationNote,
		job.Title,
		job.Company,
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

// ----- Email drafting -----

// DraftEmail generates a job application email in the language of the posting.
// Uses the candidate summary (not full profile) to keep the prompt fast.
func (m *Matcher) DraftEmail(ctx context.Context, job *models.Job, profile string) (EmailDraft, error) {
	idx := BuildCandidateIndex(profile)

	prompt := fmt.Sprintf(`You are writing a professional job application email on behalf of the candidate below.

Rules:
- Write in the SAME language as the job description (Dutch if Dutch, English if English).
- 3–4 short paragraphs: brief intro, why you fit, 1–2 specific relevant achievements, closing with call to action.
- Be specific — reference the job title and company, plus 1–2 concrete things from the candidate's background that match.
- Do NOT use generic filler ("I am very motivated", "I look forward to hearing from you") without substance.
- Use the candidate's actual name: Bo Claes. Never use placeholders like [Your Name].
- Mention naturally — not boastfully — that this job was discovered through a personal job-scouting tool Bo built (a Go backend that scrapes job boards, scores postings with a local LLM, and surfaces the best matches).
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
		Model:    m.model,
		Messages: []ollamaMessage{{Role: "user", Content: prompt}},
		Stream:   false,
		Format:   "json",
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

// ----- HTTP plumbing -----

type ollamaRequest struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Stream   bool            `json:"stream"`
	Format   string          `json:"format"`
	Options  ollamaOptions   `json:"options,omitempty"`
}

type ollamaOptions struct {
	Temperature float64 `json:"temperature,omitempty"`
	NumPredict  int     `json:"num_predict,omitempty"`
}

type ollamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaResponse struct {
	Message ollamaMessage `json:"message"`
}

// shared HTTP client with sensible timeouts. Per-call deadline still comes
// from the context passed in by the caller.
var ollamaHTTP = &http.Client{
	Timeout: 0, // rely on context
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
