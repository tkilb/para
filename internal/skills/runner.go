package skills

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/tkilb/para/internal/digest"
	"github.com/tkilb/para/internal/intake"
	"github.com/tkilb/para/internal/store"
	"github.com/tkilb/para/internal/undo"
)

const (
	timestampLayout = "2006-01-02 15-04-05"
	dateLayout      = "2006-01-02"
	defaultTimeout  = 5 * time.Minute
	killGrace       = time.Second
	maxOutputBytes  = 4 << 20
	maxNameTries    = 1000
)

var (
	// ErrOutputTooLarge reports an agent output exceeding maxOutputBytes.
	ErrOutputTooLarge = errors.New("agent output is too large")
)

// NoteResult describes a note created by running a skill in ModeNote.
type NoteResult struct {
	// Rel is the new note's path relative to the notebook root.
	Rel string
	// Action undoes (and redoes) the note creation.
	Action undo.Action
}

// Runner manages the invocation of detected agents with skills.
type Runner struct {
	Agent   Agent
	BinPath string
	Timeout time.Duration
}

// NewRunner creates a Runner with the detected agent.
func NewRunner() (Runner, error) {
	agent, path, err := DetectAgent()
	if err != nil {
		return Runner{}, err
	}
	return Runner{
		Agent:   agent,
		BinPath: path,
		Timeout: defaultTimeout,
	}, nil
}

const noteSystemInstruction = `You are generating content for a markdown note in a personal note system.
Output ONLY the final note body text.
Do NOT use tools to create, write, or modify files on disk. If the user instructions say to create, write, or save a file, produce that content directly in your text response.
Do NOT include conversational preambles or meta-commentary (such as "Here is the note:" or "I have created...").
Do NOT wrap the entire response in an outer markdown code block fence.`

// BuildPrompt combines the non-interactive system rules, skill instructions,
// and note context into a prompt for ModeNote.
func BuildPrompt(s Skill, noteName, noteContent string) string {
	var buf strings.Builder
	buf.WriteString(noteSystemInstruction)
	buf.WriteString("\n\nSkill Instructions:\n")
	buf.WriteString(s.Body)
	if strings.TrimSpace(noteContent) != "" || strings.TrimSpace(noteName) != "" {
		buf.WriteString("\n\n---\n")
		buf.WriteString(fmt.Sprintf("Note: %s\n", noteName))
		buf.WriteString("---\n")
		buf.WriteString(noteContent)
	}
	return buf.String()
}

// BuildInteractivePrompt combines the skill instructions and note context for
// an interactive CLI session (ModeInteractive), without the non-interactive
// tool/conversation restrictions.
func BuildInteractivePrompt(s Skill, noteRel, noteContent string) string {
	var buf strings.Builder
	buf.WriteString(s.Body)
	if strings.TrimSpace(noteRel) != "" || strings.TrimSpace(noteContent) != "" {
		buf.WriteString("\n\n---\n")
		buf.WriteString(fmt.Sprintf("Note: %s\n", noteRel))
		buf.WriteString("---\n")
		buf.WriteString(noteContent)
	}
	return buf.String()
}

// BuildImportPrompt combines a custom ModeImport skill body with the built-in
// import prompt contract so that short skill instructions still produce the
// JSON schema expected by the digest parser.
func BuildImportPrompt(defaultPrompt, skillBody string) string {
	body := strings.TrimSpace(skillBody)
	if body == "" {
		return defaultPrompt
	}
	// If the skill body already defines the JSON response contract, use it whole.
	if strings.Contains(body, `"notes"`) && strings.Contains(body, `"topic"`) {
		return body
	}
	return strings.TrimSpace(defaultPrompt) + "\n\nAdditional Skill Instructions:\n" + body
}

// InteractiveCmd constructs an *exec.Cmd to run an interactive session in dir,
// pre-fed with prompt instructions.
func (r Runner) InteractiveCmd(dir, prompt string) *exec.Cmd {
	var cmd *exec.Cmd
	switch r.Agent {
	case AgentAGY:
		// agy -i "<prompt>" starts interactive session with the prompt
		cmd = exec.Command(r.BinPath, "-i", prompt)
	case AgentClaude:
		// claude "<prompt>" starts interactive session with the initial prompt
		cmd = exec.Command(r.BinPath, prompt)
	default:
		cmd = exec.Command(r.BinPath, prompt)
	}
	cmd.Dir = dir
	return cmd
}

// NonInteractiveArgs returns the CLI arguments for running a single turn in print mode.
func (r Runner) NonInteractiveArgs(prompt string) []string {
	switch r.Agent {
	case AgentAGY:
		return []string{"-p", prompt, "--disable-slash-commands"}
	case AgentClaude:
		return []string{"-p", prompt, "--tools", "", "--disable-slash-commands"}
	default:
		return []string{"-p", prompt}
	}
}

// RunNote executes the skill non-interactively against a note, and writes the result
// to a new note with a datetime suffix in the same folder as the original note.
// Returns the relative path of the new note.
func (r Runner) RunNote(ctx context.Context, notebookRoot, relNotePath, noteContent string, s Skill, now time.Time) (string, error) {
	res, err := r.RunNoteInStore(ctx, store.NewMarkdown(notebookRoot), notebookRoot, relNotePath, noteContent, s, now)
	return res.Rel, err
}

// RunNoteInStore executes the skill non-interactively against a note and creates
// the timestamped output note via st.Create, returning the relative path and undo.Action.
func (r Runner) RunNoteInStore(ctx context.Context, st store.Store, notebookRoot, relNotePath, noteContent string, s Skill, now time.Time) (NoteResult, error) {
	if strings.TrimSpace(s.Body) == "" {
		return NoteResult{}, ErrEmptyBody
	}
	prompt := BuildPrompt(s, filepath.Base(relNotePath), noteContent)

	timeout := r.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	args := r.NonInteractiveArgs(prompt)
	cmd := exec.CommandContext(runCtx, r.BinPath, args...)
	cmd.WaitDelay = killGrace

	tmpDir, err := os.MkdirTemp("", "para-skill-")
	if err == nil {
		defer os.RemoveAll(tmpDir)
		cmd.Dir = tmpDir
	} else {
		cmd.Dir = notebookRoot
	}

	var stdout, stderr limitedBuffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	if ctx.Err() != nil {
		return NoteResult{}, ctx.Err()
	}
	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		return NoteResult{}, fmt.Errorf("agent timed out: %w", runCtx.Err())
	}
	if stdout.over {
		return NoteResult{}, ErrOutputTooLarge
	}
	if runErr != nil {
		errMsg := strings.TrimSpace(stderr.buf.String())
		if errMsg != "" {
			return NoteResult{}, fmt.Errorf("agent error: %s (%w)", errMsg, runErr)
		}
		return NoteResult{}, fmt.Errorf("agent failed: %w", runErr)
	}

	resultText := stripOuterMarkdownFence(strings.TrimSpace(stdout.buf.String()))
	if resultText == "" {
		return NoteResult{}, errors.New("agent returned empty response")
	}

	dir := filepath.Dir(relNotePath)
	base := filepath.Base(relNotePath)
	stem := stemOf(base)
	ts := now.Format(timestampLayout)
	baseStem := fmt.Sprintf("%s %s", stem, ts)

	origNote := store.ParseNote(base, []byte(noteContent))
	var origSource string
	var origTags []string
	var origPeople []string
	if origNote.Frontmatter != nil {
		origSource = getScalar(origNote.Frontmatter, "source")
		origTags = getList(origNote.Frontmatter, "tags")
		origPeople = getList(origNote.Frontmatter, "people")
	}

	for n := 1; n <= maxNameTries; n++ {
		candidateBase := baseStem + ext
		if n > 1 {
			candidateBase = fmt.Sprintf("%s %d%s", baseStem, n, ext)
		}
		var candidateRel string
		if dir == "." || dir == "" {
			candidateRel = candidateBase
		} else {
			candidateRel = filepath.Join(dir, candidateBase)
		}

		taken, err := st.Exists(candidateRel)
		if err != nil {
			return NoteResult{}, err
		}
		if taken {
			continue
		}

		noteBytes, err := store.IntakeNoteWithPeople(candidateBase, origNote.Kind, origSource, now.Format(dateLayout), resultText+"\n", origTags, origPeople)
		if err != nil {
			return NoteResult{}, fmt.Errorf("build note: %w", err)
		}

		act, err := st.Create(candidateRel, noteBytes)
		if errors.Is(err, store.ErrExists) {
			continue
		}
		if err != nil {
			return NoteResult{}, fmt.Errorf("write note: %w", err)
		}
		return NoteResult{Rel: candidateRel, Action: act}, nil
	}

	return NoteResult{}, intake.ErrNoFreeName
}

// stripOuterMarkdownFence removes a single outer ```markdown ... ``` or ``` ... ```
// fence when the LLM wrapped its entire note response in a code block.
func stripOuterMarkdownFence(s string) string {
	lines := strings.Split(s, "\n")
	if len(lines) < 2 {
		return s
	}
	first := strings.TrimSpace(lines[0])
	last := strings.TrimSpace(lines[len(lines)-1])
	if last != "```" {
		return s
	}
	switch strings.ToLower(first) {
	case "```", "```md", "```markdown", "```json":
		// Only strip if there is no other closing ``` fence inside.
		for _, mid := range lines[1 : len(lines)-1] {
			if strings.TrimSpace(mid) == "```" {
				return s
			}
		}
		return strings.TrimSpace(strings.Join(lines[1:len(lines)-1], "\n"))
	}
	return s
}

// AGYDigester is a digest.Digester that runs the agy CLI in print mode inside
// an isolated temporary directory and parses the resulting JSON via digest.Parse.
type AGYDigester struct {
	BinPath string
	Timeout time.Duration
	Prompt  string
	Sources []string
}

// Digest runs the agy CLI over source and returns its checked answer.
func (d AGYDigester) Digest(ctx context.Context, source string) (digest.Digest, error) {
	if strings.TrimSpace(d.BinPath) == "" || strings.TrimSpace(d.Prompt) == "" || d.Timeout <= 0 {
		return digest.Digest{}, digest.ErrBadSettings
	}
	if strings.TrimSpace(source) == "" {
		return digest.Digest{}, digest.ErrEmptySource
	}
	dir, err := os.MkdirTemp("", "para-digest-")
	if err != nil {
		return digest.Digest{}, fmt.Errorf("%w: no temporary directory", digest.ErrBackendFailed)
	}
	defer os.RemoveAll(dir)

	runCtx, cancel := context.WithTimeout(ctx, d.Timeout)
	defer cancel()

	fullPrompt := d.Prompt + "\n\n---\nSource:\n---\n" + source
	cmd := exec.CommandContext(runCtx, d.BinPath, "-p", fullPrompt, "--disable-slash-commands", "--json-schema", digest.Schema)
	cmd.Dir = dir
	cmd.WaitDelay = killGrace
	var stdout limitedBuffer
	cmd.Stdout = &stdout

	runErr := cmd.Run()
	switch {
	case ctx.Err() != nil:
		return digest.Digest{}, ctx.Err()
	case errors.Is(runCtx.Err(), context.DeadlineExceeded):
		return digest.Digest{}, digest.ErrTimeout
	case stdout.over:
		return digest.Digest{}, digest.ErrTooLarge
	case runErr != nil:
		var exit *exec.ExitError
		if errors.As(runErr, &exit) {
			return digest.Digest{}, fmt.Errorf("%w (exit %d)", digest.ErrBackendFailed, exit.ExitCode())
		}
		return digest.Digest{}, digest.ErrBackendMissing
	}

	raw := strings.TrimSpace(stripOuterMarkdownFence(strings.TrimSpace(stdout.buf.String())))
	if raw == "" {
		return digest.Digest{}, digest.ErrNoOutput
	}
	return digest.Parse([]byte(raw), d.Sources)
}

// Merge implements digest.Merger by marshaling partial digests and running Digest.
func (d AGYDigester) Merge(ctx context.Context, parts []digest.Digest) (digest.Digest, error) {
	data, err := digest.MarshalParts(parts)
	if err != nil {
		return digest.Digest{}, fmt.Errorf("%w: encode the parts", digest.ErrBackendFailed)
	}
	return d.Digest(ctx, string(data))
}

type limitedBuffer struct {
	buf  bytes.Buffer
	over bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	room := maxOutputBytes - b.buf.Len()
	if len(p) > room {
		b.over = true
		p = p[:max(room, 0)]
	}
	b.buf.Write(p)
	return len(p), nil
}

var _ io.Writer = (*limitedBuffer)(nil)

func getScalar(fm *store.Frontmatter, key string) string {
	if fm == nil {
		return ""
	}
	if v := fm.Get(key); v != nil {
		return strings.TrimSpace(v.Value)
	}
	return ""
}

func getList(fm *store.Frontmatter, key string) []string {
	if fm == nil {
		return nil
	}
	v := fm.Get(key)
	if v == nil {
		return nil
	}
	var out []string
	items := v.Content
	if len(items) == 0 && strings.TrimSpace(v.Value) != "" {
		items = append(items, v)
	}
	for _, it := range items {
		if s := strings.TrimSpace(it.Value); s != "" {
			out = append(out, s)
		}
	}
	return out
}
