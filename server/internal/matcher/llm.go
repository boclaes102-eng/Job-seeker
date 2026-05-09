package matcher

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
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

	reason := strings.TrimSpace(result.Reason)
	if reason == "" {
		reason = det.Reason
	}

	// The model sometimes writes "80/100 —" in the reason but a different
	// number in the score field. Parse the leading score from the reason and
	// use that as the authoritative value — it's what the model committed to
	// in plain text before producing the JSON.
	if n := parseLeadingScore(reason); n >= 0 {
		result.Score = n
	}

	// Sanity-clamp: refuse adjustments greater than ±25 points from the
	// deterministic anchor. The LLM is a referee, not a re-scorer.
	final := clampAdjustment(det.Total, result.Score, 25)
	return final, reason, nil
}

// parseLeadingScore extracts the score from a reason string that starts with
// "72/100 — " or "72/100: " etc. Returns -1 if no such prefix is found.
func parseLeadingScore(reason string) int {
	// Look for a number immediately followed by "/100"
	idx := strings.Index(reason, "/100")
	if idx <= 0 {
		return -1
	}
	// Walk backwards to find the start of the number
	start := idx - 1
	for start > 0 && reason[start-1] >= '0' && reason[start-1] <= '9' {
		start--
	}
	n, err := strconv.Atoi(reason[start:idx])
	if err != nil || n < 0 || n > 100 {
		return -1
	}
	return n
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

	// NOTE: we intentionally do NOT pass the full candidate tech list here.
	// Passing it caused the LLM to do its own semantic matching against the
	// job description (e.g. "Azure OpenAI ≈ Anthropic Claude"), inventing
	// overlaps the deterministic scorer never confirmed.
	// The LLM's only job here is to judge IMPORTANCE of already-found matches,
	// not to discover new ones.
	return fmt.Sprintf(`You are adjusting a job-match score. The tech matching is already done — your only job is to judge whether the confirmed matches are PRIMARY requirements of this job, and whether the missing techs are deal-breakers.

Job: %s at %s
Baseline: %d/100
Role match: %s

CONFIRMED MATCHES (candidate has these AND they appear in the job description):
%s

JOB REQUIRES BUT CANDIDATE LACKS:
%s

Job description (use ONLY to judge if matched/missing techs are primary or secondary requirements):
%s

Rules:
- Adjust UP (max +25) if the CONFIRMED MATCHES are core/primary requirements of this job.
- Adjust DOWN (max -25) if the JOB REQUIRES items are the primary focus and candidate clearly lacks them; or the domain is completely wrong (mechanical, pharma, accounting).
- Keep the baseline if uncertain or if matches and gaps are balanced.
- Do NOT identify any tech overlaps beyond CONFIRMED MATCHES above. Do NOT reference any technology not in those two lists.
- Do NOT factor in location.

The reason MUST start with the score you chose, e.g. "72/100 — ".
Return ONLY valid JSON:
{"score": integer 0-100, "reason": "<score>/100 — one sentence referencing only techs from the two lists above"}`,
		job.Title, job.Company,
		det.Total,
		roleMatch,
		matched,
		missing,
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
	score := DeterministicScore(job, idx)

	// Pick the experience entry most relevant to this job.
	bestExp := pickBestExperience(profile, job.Title+" "+job.Description)

	// Ask the LLM for ONE sentence only: how the matched tech connects to this role.
	// Keeping the ask tiny reduces hallucination to near-zero.
	fitSentence, err := m.generateFitSentence(ctx, job.Title, job.Company, score.MatchedTech)
	if err != nil {
		// Fall back to a safe generic sentence — don't fail the whole email.
		fitSentence = fmt.Sprintf("My background in %s aligns directly with the requirements listed for this role.",
			joinTech(score.MatchedTech, 3))
	}

	subject := fmt.Sprintf("Application: %s — Bo Claes", job.Title)
	body := buildEmailBody(job.Title, job.Company, fitSentence, bestExp)
	return EmailDraft{Subject: subject, Body: body}, nil
}

// generateFitSentence asks the LLM for a single sentence explaining the tech match.
// The prompt is so constrained that hallucination is nearly impossible.
func (m *Matcher) generateFitSentence(ctx context.Context, title, company string, matchedTech []string) (string, error) {
	if len(matchedTech) == 0 {
		return "", fmt.Errorf("no matched tech")
	}
	tech := joinTech(matchedTech, 4)
	prompt := fmt.Sprintf(
		`Write exactly ONE sentence (max 25 words) explaining how experience with %s is relevant to a %s role at %s. Use only these technologies — do not mention any others. Return plain text, no quotes.`,
		tech, title, company,
	)
	req := ollamaRequest{
		Model:    m.model,
		Messages: []ollamaMessage{{Role: "user", Content: prompt}},
		Stream:   false,
		Options:  ollamaOptions{Temperature: 0.3, NumPredict: 60},
	}
	var resp ollamaResponse
	if err := m.callOllama(ctx, "/api/chat", req, &resp); err != nil {
		return "", err
	}
	s := strings.TrimSpace(resp.Message.Content)
	s = strings.Trim(s, `"`)
	if s == "" {
		return "", fmt.Errorf("empty response")
	}
	return s, nil
}

// pickBestExperience returns the experience entry whose text overlaps most with
// the job's combined title+description. Falls back to the first entry.
func pickBestExperience(profile, jobText string) string {
	expSection := sectionContent(profile, "Experience")
	if expSection == "" {
		return ""
	}
	// Split into individual bullet entries on lines starting with "- **"
	var entries []string
	current := ""
	for _, line := range strings.Split(expSection, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "- **") && current != "" {
			entries = append(entries, strings.TrimSpace(current))
			current = line
		} else {
			if current != "" {
				current += "\n" + line
			} else {
				current = line
			}
		}
	}
	if current != "" {
		entries = append(entries, strings.TrimSpace(current))
	}
	if len(entries) == 0 {
		return strings.TrimSpace(expSection)
	}

	jobWords := wordSet(strings.ToLower(jobText))
	best, bestScore := entries[0], -1
	for _, e := range entries {
		score := 0
		for w := range wordSet(strings.ToLower(e)) {
			if len(w) > 3 && jobWords[w] {
				score++
			}
		}
		if score > bestScore {
			bestScore = score
			best = e
		}
	}
	// Strip the leading "- " bullet marker and markdown bold markers (**text**).
	best = strings.TrimPrefix(strings.TrimSpace(best), "- ")
	best = strings.ReplaceAll(best, "**", "")
	return best
}

func buildEmailBody(title, company, fitSentence, bestExp string) string {
	var b strings.Builder
	b.WriteString("Dear Hiring Manager,\n\n")

	// Paragraph 1: intro + how the role was found
	fmt.Fprintf(&b, "I am writing to apply for the %s position at %s. I came across this role via a personal job-scouting tool I built — a Go backend that scrapes job boards and scores postings with a local LLM to surface best matches.\n\n", title, company)

	// Paragraph 2: skill fit (LLM-generated single sentence)
	b.WriteString(fitSentence)
	b.WriteString("\n\n")

	// Paragraph 3: most relevant experience entry (verbatim from profile — no hallucination)
	if bestExp != "" {
		b.WriteString(bestExp)
		b.WriteString("\n\n")
	}

	// Paragraph 4: call to action
	b.WriteString("I would welcome the opportunity to discuss how my background fits your needs.\n\n")

	b.WriteString("Bo Claes | boclaes102@gmail.com | github.com/boclaes102-eng")
	return b.String()
}

func joinTech(tech []string, max int) string {
	if len(tech) == 0 {
		return "my technical skills"
	}
	if len(tech) > max {
		tech = tech[:max]
	}
	if len(tech) == 1 {
		return tech[0]
	}
	return strings.Join(tech[:len(tech)-1], ", ") + " and " + tech[len(tech)-1]
}

func wordSet(s string) map[string]bool {
	m := map[string]bool{}
	for _, w := range strings.FieldsFunc(s, func(r rune) bool {
		return !('a' <= r && r <= 'z') && !('0' <= r && r <= '9')
	}) {
		m[w] = true
	}
	return m
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
