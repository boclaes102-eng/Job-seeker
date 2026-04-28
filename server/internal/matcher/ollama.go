package matcher

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"job-seeker/server/internal/models"
)

type MatchResult struct {
	Score  int    `json:"score"`
	Reason string `json:"reason"`
}

type Matcher struct {
	host  string
	model string
}

func New() *Matcher {
	host := os.Getenv("OLLAMA_HOST")
	if host == "" {
		host = "http://localhost:11434"
	}
	model := os.Getenv("OLLAMA_MODEL")
	if model == "" {
		model = "llama3.1:8b"
	}
	return &Matcher{host: host, model: model}
}

type ollamaRequest struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Stream   bool            `json:"stream"`
	Format   string          `json:"format"`
}

type ollamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaResponse struct {
	Message ollamaMessage `json:"message"`
}

type EmailDraft struct {
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

// DraftEmail generates a professional job application email in the language of the job description.
func (m *Matcher) DraftEmail(ctx context.Context, job *models.Job, profile string) (EmailDraft, error) {
	summary := buildCandidateSummary(profile)

	prompt := fmt.Sprintf(`You are writing a professional job application email on behalf of the candidate below.

Rules:
- Write in the SAME language as the job description (Dutch if the job is in Dutch, English if English)
- Keep it to 3–4 short paragraphs: brief intro, why you fit, 1–2 specific relevant achievements, closing with call to action
- Be specific — reference the job title and company name, and 1–2 concrete things from the candidate's background that match
- Do NOT use generic filler phrases like "I am very motivated" or "I look forward to hearing from you" without substance
- Do NOT include placeholder text like [Your Name] — use the candidate's actual name: Bo Claes
- Sign off with: Bo Claes | boclaes102@gmail.com | github.com/boclaes102-eng

Job title: %s
Company: %s
Job description:
%s

Candidate summary:
%s

Return ONLY valid JSON with exactly two fields:
{"subject": "short professional subject line", "body": "full email body as plain text with newlines as \\n"}`,
		job.Title, job.Company,
		truncate(job.Description, 1500),
		summary)

	body, _ := json.Marshal(ollamaRequest{
		Model:    m.model,
		Messages: []ollamaMessage{{Role: "user", Content: prompt}},
		Stream:   false,
		Format:   "json",
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.host+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return EmailDraft{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return EmailDraft{}, fmt.Errorf("ollama: %w", err)
	}
	defer resp.Body.Close()

	var ollamaResp ollamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return EmailDraft{}, fmt.Errorf("ollama decode: %w", err)
	}

	var draft EmailDraft
	if err := json.Unmarshal([]byte(ollamaResp.Message.Content), &draft); err != nil {
		return EmailDraft{}, fmt.Errorf("draft parse: %w", err)
	}
	return draft, nil
}

func (m *Matcher) Score(ctx context.Context, job *models.Job, profile string) (int, string, error) {
	if profile == "" {
		profile = "(no profile provided)"
	}

	summary := buildCandidateSummary(profile)

	prompt := fmt.Sprintf(`You are a precise job-fit evaluator. The job description may be in Dutch or English — evaluate it accurately either way.

Step 1: Read the job and identify the PRIMARY required technologies/domain.
Step 2: Check the candidate summary for each requirement.
Step 3: Score honestly.

SCORING:
- 85-100: candidate has ALL or nearly all primary required technologies
- 70-84: candidate has most, 1 minor gap
- 50-69: candidate has some, 1 significant gap (e.g. has React but job needs Angular)
- 30-49: missing a core required technology (e.g. job needs .NET, candidate has none)
- 0-29: wrong domain or stack entirely

STRICT RULES:
- Score based on PRIMARY tech the JOB requires, not the candidate's general impressiveness
- Angular ≠ React, .NET ≠ Node.js — treat as gaps
- Only mention security/IoT/hardware experience if the JOB explicitly requires it
- Do not mention unrelated projects to justify a high score

Job title: %s
Company: %s
Job description:
%s

Candidate summary:
%s

Return ONLY valid JSON:
{"score": integer, "reason": "2-3 sentences naming the primary required technologies and whether the candidate has them. Be specific and factual."}`,
		job.Title, job.Company,
		truncate(job.Description, 2000),
		summary)

	body, _ := json.Marshal(ollamaRequest{
		Model:    m.model,
		Messages: []ollamaMessage{{Role: "user", Content: prompt}},
		Stream:   false,
		Format:   "json",
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.host+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, "", fmt.Errorf("ollama: %w", err)
	}
	defer resp.Body.Close()

	var ollamaResp ollamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return 0, "", fmt.Errorf("ollama decode: %w", err)
	}

	var result MatchResult
	if err := json.Unmarshal([]byte(ollamaResp.Message.Content), &result); err != nil {
		return 0, ollamaResp.Message.Content, fmt.Errorf("result parse: %w", err)
	}
	return result.Score, result.Reason, nil
}

// buildCandidateSummary produces a compact but complete candidate summary from profile.md.
// Each section is condensed to its first meaningful line so the model gets full context
// without anchoring on any single domain (e.g. cybersecurity narrative).
func buildCandidateSummary(profile string) string {
	sections := map[string]string{
		"About me":             "",
		"Experience":           "",
		"Education":            "",
		"Projects":             "",
		"Tech stack":           "",
		"What I'm looking for": "",
	}
	order := []string{"Tech stack", "What I'm looking for", "Experience", "Projects", "Education", "About me"}

	// Extract each section
	for heading := range sections {
		sections[heading] = condensedSection(extractSection(profile, heading))
	}

	var sb strings.Builder
	for _, key := range order {
		if v := sections[key]; v != "" {
			sb.WriteString("## " + key + "\n" + v + "\n\n")
		}
	}
	return strings.TrimSpace(sb.String())
}

// condensedSection keeps only lines that carry factual content (not blank, not pure narrative).
// Caps at ~400 chars so each section stays short.
func condensedSection(s string) string {
	var lines []string
	total := 0
	for _, line := range strings.Split(strings.TrimSpace(s), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "<!--") {
			continue
		}
		// Skip pure narrative sentences (long prose without tech keywords)
		lines = append(lines, line)
		total += len(line)
		if total > 400 {
			break
		}
	}
	return strings.Join(lines, "\n")
}

// extractSection pulls the content under a ## Heading from markdown.
func extractSection(md, heading string) string {
	lines := strings.Split(md, "\n")
	var result []string
	inSection := false
	for _, line := range lines {
		if strings.HasPrefix(line, "## ") {
			if inSection {
				break
			}
			if strings.EqualFold(strings.TrimPrefix(line, "## "), heading) {
				inSection = true
			}
			continue
		}
		if inSection {
			result = append(result, line)
		}
	}
	return strings.Join(result, "\n")
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
