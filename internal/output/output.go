package output

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/mattn/go-isatty"
)

// global mode flags set from CLI flags in PersistentPreRun
var jsonMode bool
var noInput bool

// SetJSONMode activates machine-readable JSON output mode
func SetJSONMode(v bool) { jsonMode = v }

// IsJSONMode returns true if --json flag is active
func IsJSONMode() bool { return jsonMode }

// SetNoInput activates non-interactive mode (no prompts)
func SetNoInput(v bool) { noInput = v }

// IsNonInteractive returns true when running in CI, non-TTY, or --no-input mode
func IsNonInteractive() bool {
	return noInput || !IsTTY()
}

// IsTTY returns true if stdout is a terminal and not in CI/no-color mode
func IsTTY() bool {
	if os.Getenv("CI") == "true" || os.Getenv("NO_COLOR") != "" {
		return false
	}
	return isatty.IsTerminal(os.Stdout.Fd())
}

// globalJQ and globalFields are set from CLI flags before rendering
var globalJQ string
var globalFields []string

// SetGlobalFilters sets jq expression and fields for the current invocation
func SetGlobalFilters(jq string, fields []string) {
	globalJQ = jq
	globalFields = fields
}

// JSON prints v as JSON to stdout, applying global --jq and --fields filters
func JSON(v any) error {
	if globalJQ != "" || len(globalFields) > 0 {
		return JSONWithFilter(v, globalJQ, globalFields)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// Table prints rows as aligned columns to stdout using tabwriter
// headers: column names; rows: [][]string
func Table(headers []string, rows [][]string) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, strings.Join(headers, "\t"))
	fmt.Fprintln(w, strings.Repeat("-\t", len(headers)))
	for _, row := range rows {
		fmt.Fprintln(w, strings.Join(row, "\t"))
	}
	w.Flush()
}

// KV prints key-value pairs
func KV(pairs map[string]string) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for k, v := range pairs {
		fmt.Fprintf(w, "%s\t%s\n", k, v)
	}
	w.Flush()
}

// Success prints a success message to stderr
func Success(msg string) {
	fmt.Fprintln(os.Stderr, "✓ "+msg)
}

// Info prints an info message to stderr
func Info(msg string) {
	fmt.Fprintln(os.Stderr, msg)
}

// Warn prints a warning message to stderr
func Warn(msg string) {
	fmt.Fprintln(os.Stderr, "⚠  "+msg)
}

// Err prints an error to stderr with formatting
func Err(what, why string, next ...string) {
	fmt.Fprintf(os.Stderr, "Error: %s\n", what)
	if why != "" {
		fmt.Fprintf(os.Stderr, "\n%s\n", why)
	}
	if len(next) > 0 {
		fmt.Fprintln(os.Stderr)
		for i := 0; i < len(next)-1; i += 2 {
			cmd := next[i]
			desc := ""
			if i+1 < len(next) {
				desc = next[i+1]
			}
			fmt.Fprintf(os.Stderr, "  %-40s %s\n", cmd, desc)
		}
	}
}

// JSONErrorPayload is the structured error output when --json is active
type JSONErrorPayload struct {
	Error      bool   `json:"error"`
	Code       string `json:"code,omitempty"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion,omitempty"`
}

// PrintJSONError writes a structured JSON error to stdout (used when --json is active)
func PrintJSONError(code, message, suggestion string) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(JSONErrorPayload{
		Error:      true,
		Code:       code,
		Message:    message,
		Suggestion: suggestion,
	})
}

// NDJSONLine writes one JSON line to stdout (for streaming)
func NDJSONLine(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = fmt.Printf("%s\n", b)
	return err
}

// JSONWithFilter prints v as JSON, optionally filtered by jq expression or fields.
func JSONWithFilter(v any, jqExpr string, fields []string) error {
	if jqExpr == "" && len(fields) == 0 {
		return JSON(v)
	}
	// Marshal to bytes first
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	var raw any
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	// Apply --fields before --jq
	if len(fields) > 0 {
		raw = applyFields(raw, fields)
	}
	if jqExpr != "" {
		raw, err = applyJQ(raw, jqExpr)
		if err != nil {
			return fmt.Errorf("jq: %w", err)
		}
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(raw)
}

func applyFields(v any, fields []string) any {
	fieldSet := make(map[string]bool, len(fields))
	for _, f := range fields {
		fieldSet[strings.TrimSpace(f)] = true
	}
	switch val := v.(type) {
	case map[string]any:
		out := make(map[string]any)
		for _, f := range fields {
			if x, ok := val[f]; ok {
				out[f] = x
			}
		}
		return out
	case []any:
		result := make([]any, 0, len(val))
		for _, item := range val {
			result = append(result, applyFields(item, fields))
		}
		return result
	default:
		_ = fieldSet
		return v
	}
}

func applyJQ(v any, expr string) (any, error) {
	// Import is handled at package level — add to imports below
	return applyGojq(v, expr)
}
