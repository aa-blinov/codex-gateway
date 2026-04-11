package main

import (
	"os"
	"strings"
	"testing"
)

// ── parseModelAndPreset ───────────────────────────────────────────────────────

func TestParseModelAndPreset(t *testing.T) {
	tests := []struct {
		input       any
		wantModel   string
		wantPreset  string
		wantErr     bool
	}{
		{"gpt-5.3-codex-spark", "gpt-5.3-codex-spark", "", false},
		{"gpt-5.3-codex-spark@fast", "gpt-5.3-codex-spark", "fast", false},
		{"@preset", "", "", true},
		{"model@", "", "", true},
		{nil, "", "", true},
		{"", "", "", true},
		{42, "", "", true},
	}
	for _, tc := range tests {
		model, preset, err := parseModelAndPreset(tc.input)
		if (err != nil) != tc.wantErr {
			t.Errorf("parseModelAndPreset(%v): wantErr=%v got %v", tc.input, tc.wantErr, err)
			continue
		}
		if !tc.wantErr && (model != tc.wantModel || preset != tc.wantPreset) {
			t.Errorf("parseModelAndPreset(%v): got (%q, %q), want (%q, %q)",
				tc.input, model, preset, tc.wantModel, tc.wantPreset)
		}
	}
}

// ── parseReasoningEffort ──────────────────────────────────────────────────────

func TestParseReasoningEffort(t *testing.T) {
	tests := []struct {
		body    map[string]any
		want    string
		wantErr bool
	}{
		{map[string]any{}, "", false},
		{map[string]any{"reasoning_effort": "low"}, "low", false},
		{map[string]any{"reasoning_effort": "medium"}, "medium", false},
		{map[string]any{"reasoning_effort": "high"}, "high", false},
		{map[string]any{"reasoning": map[string]any{"effort": "low"}}, "low", false},
		{map[string]any{"reasoning_effort": "invalid"}, "", true},
		{map[string]any{"reasoning_effort": "low", "reasoning": map[string]any{"effort": "low"}}, "", true},
		{map[string]any{"reasoning": "not-an-object"}, "", true},
	}
	for _, tc := range tests {
		got, err := parseReasoningEffort(tc.body)
		if (err != nil) != tc.wantErr {
			t.Errorf("parseReasoningEffort(%v): wantErr=%v got %v", tc.body, tc.wantErr, err)
			continue
		}
		if !tc.wantErr && got != tc.want {
			t.Errorf("parseReasoningEffort(%v): got %q, want %q", tc.body, got, tc.want)
		}
	}
}

// ── validateChatRequest ───────────────────────────────────────────────────────

func TestValidateChatRequest(t *testing.T) {
	tests := []struct {
		body    map[string]any
		wantErr bool
	}{
		{map[string]any{}, false},
		{map[string]any{"n": float64(1)}, false},
		{map[string]any{"stream": true}, true},
		{map[string]any{"n": float64(2)}, true},
		{map[string]any{"tools": []any{}}, true},
		{map[string]any{"temperature": 0.7}, true},
	}
	for _, tc := range tests {
		err := validateChatRequest(tc.body)
		if (err != nil) != tc.wantErr {
			t.Errorf("validateChatRequest(%v): wantErr=%v got %v", tc.body, tc.wantErr, err)
		}
	}
}

// ── normalizeContent ──────────────────────────────────────────────────────────

func TestNormalizeContent(t *testing.T) {
	got, err := normalizeContent("user", "hello")
	if err != nil || got != "hello" {
		t.Errorf("string content: got %q, %v", got, err)
	}

	parts := []any{
		map[string]any{"type": "text", "text": "foo"},
		map[string]any{"type": "text", "text": "bar"},
	}
	got, err = normalizeContent("user", parts)
	if err != nil || got != "foo\nbar" {
		t.Errorf("list content: got %q, %v", got, err)
	}

	_, err = normalizeContent("user", map[string]any{"type": "text", "text": "x"})
	if err == nil {
		t.Error("object content: expected error")
	}

	badParts := []any{map[string]any{"type": "image_url", "text": "x"}}
	_, err = normalizeContent("user", badParts)
	if err == nil {
		t.Error("unsupported part type: expected error")
	}
}

// ── buildPrompt ───────────────────────────────────────────────────────────────

func TestBuildPrompt(t *testing.T) {
	msgs := []any{
		map[string]any{"role": "user", "content": "Say hello"},
	}
	prompt, err := buildPrompt(msgs, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(prompt, "Say hello") {
		t.Error("prompt missing user message")
	}
	if !strings.Contains(prompt, "<CONVERSATION>") {
		t.Error("prompt missing CONVERSATION block")
	}

	// system + user
	msgs2 := []any{
		map[string]any{"role": "system", "content": "Be brief."},
		map[string]any{"role": "user", "content": "Hello"},
	}
	prompt2, err := buildPrompt(msgs2, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(prompt2, "<SYSTEM_MESSAGES>") {
		t.Error("prompt missing SYSTEM_MESSAGES block")
	}

	// structured output flag adds extra instruction
	prompt3, _ := buildPrompt(msgs, true)
	if !strings.Contains(prompt3, "JSON Schema") {
		t.Error("structured output prompt missing schema instruction")
	}

	// only system messages → error (no user/assistant in transcript)
	msgsOnlySystem := []any{
		map[string]any{"role": "system", "content": "Be brief."},
	}
	_, err = buildPrompt(msgsOnlySystem, false)
	if err == nil {
		t.Error("expected error when only system messages")
	}

	// empty list → error
	_, err = buildPrompt([]any{}, false)
	if err == nil {
		t.Error("expected error for empty messages")
	}

	// unsupported role → error
	msgsBAd := []any{
		map[string]any{"role": "function", "content": "x"},
	}
	_, err = buildPrompt(msgsBAd, false)
	if err == nil {
		t.Error("expected error for unsupported role")
	}
}

// ── extractChatSchema ─────────────────────────────────────────────────────────

func TestExtractChatSchema(t *testing.T) {
	// no response_format
	s, err := extractChatSchema(map[string]any{})
	if err != nil || s != nil {
		t.Errorf("no rf: got %v, %v", s, err)
	}

	// type=text → nil schema
	s, err = extractChatSchema(map[string]any{"response_format": map[string]any{"type": "text"}})
	if err != nil || s != nil {
		t.Errorf("type=text: got %v, %v", s, err)
	}

	// valid json_schema
	validRF := map[string]any{
		"response_format": map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"schema": map[string]any{"type": "object"},
			},
		},
	}
	s, err = extractChatSchema(validRF)
	if err != nil || s == nil {
		t.Errorf("valid json_schema: got %v, %v", s, err)
	}

	// unsupported type
	_, err = extractChatSchema(map[string]any{"response_format": map[string]any{"type": "json_object"}})
	if err == nil {
		t.Error("expected error for unsupported type")
	}
}

// ── buildResponsesMessages ────────────────────────────────────────────────────

func TestBuildResponsesMessages(t *testing.T) {
	// string input
	body := map[string]any{"input": "hello"}
	msgs, err := buildResponsesMessages(body)
	if err != nil || len(msgs) != 1 {
		t.Fatalf("string input: got %v, %v", msgs, err)
	}

	// with instructions
	body2 := map[string]any{"instructions": "Be brief.", "input": "hello"}
	msgs2, err := buildResponsesMessages(body2)
	if err != nil || len(msgs2) != 2 {
		t.Fatalf("instructions: got %v, %v", msgs2, err)
	}
	first := msgs2[0].(map[string]any)
	if first["role"] != "system" {
		t.Error("first message should be system")
	}

	// missing input
	_, err = buildResponsesMessages(map[string]any{})
	if err == nil {
		t.Error("expected error for missing input")
	}
}

// ── loadModels ────────────────────────────────────────────────────────────────

func TestLoadModels(t *testing.T) {
	toml := `
model = "gpt-5.3-codex-spark"

[profiles.fast]
model = "gpt-5.3-codex-spark-fast"

[profiles.mini]
model = "gpt-5.4-mini"
`
	f, err := os.CreateTemp("", "config-*.toml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString(toml)
	f.Close()

	models := loadModels(f.Name())
	if len(models) != 3 {
		t.Fatalf("expected 3 models, got %d: %v", len(models), models)
	}

	// dedup
	tomlDup := `model = "same"\nmodel = "same"`
	f2, _ := os.CreateTemp("", "config-*.toml")
	defer os.Remove(f2.Name())
	f2.WriteString(tomlDup)
	f2.Close()

	models2 := loadModels(f2.Name())
	if len(models2) != 1 {
		t.Errorf("expected 1 unique model, got %d: %v", len(models2), models2)
	}

	// missing file → empty list, no panic
	models3 := loadModels("/nonexistent/path/config.toml")
	if models3 == nil {
		models3 = []string{}
	}
	if len(models3) != 0 {
		t.Errorf("missing file: expected empty, got %v", models3)
	}
}
