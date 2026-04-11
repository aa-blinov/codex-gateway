package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ── defaults ─────────────────────────────────────────────────────────────────

const (
	defaultHost           = "127.0.0.1"
	defaultPort           = 8000
	defaultMaxConcurrency = 100
	defaultTimeoutSeconds = 600
)

var (
	supportedReasoningEfforts = map[string]bool{"low": true, "medium": true, "high": true}
	textPartTypes             = map[string]bool{"text": true, "input_text": true, "output_text": true}
	modelLineRe               = regexp.MustCompile(`(?m)^\s*model\s*=\s*"([^"]+)"`)
)

// ── server state ──────────────────────────────────────────────────────────────

type srv struct {
	cwd            string
	configPath     string
	timeoutSeconds int
	sem            chan struct{}
}

// ── request error ─────────────────────────────────────────────────────────────

type reqErr struct {
	msg        string
	statusCode int
	code       string
}

func (e *reqErr) Error() string { return e.msg }

func badRequest(msg string) *reqErr {
	return &reqErr{msg: msg, statusCode: 400, code: "invalid_request_error"}
}

func serverError(msg string, code string) *reqErr {
	return &reqErr{msg: msg, statusCode: 500, code: code}
}

func timeoutError(msg string) *reqErr {
	return &reqErr{msg: msg, statusCode: 504, code: "timeout"}
}

// ── JSON helpers ──────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, e *reqErr) {
	writeJSON(w, e.statusCode, map[string]any{
		"error": map[string]any{"message": e.msg, "type": e.code, "code": e.code},
	})
}

func randHex() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ── validation helpers ────────────────────────────────────────────────────────

func requireString(v any, field string) (string, error) {
	s, ok := v.(string)
	if !ok || strings.TrimSpace(s) == "" {
		return "", badRequest(fmt.Sprintf("`%s` must be a non-empty string.", field))
	}
	return s, nil
}

func parseModelAndPreset(raw any) (model, preset string, err error) {
	name, err := requireString(raw, "model")
	if err != nil {
		return "", "", err
	}
	if !strings.Contains(name, "@") {
		return name, "", nil
	}
	parts := strings.SplitN(name, "@", 2)
	if parts[0] == "" || parts[1] == "" {
		return "", "", badRequest("`model` must be either `model_name` or `model_name@preset_name`.")
	}
	return parts[0], parts[1], nil
}

func parseReasoningEffort(body map[string]any) (string, error) {
	direct := body["reasoning_effort"]
	nested := body["reasoning"]
	if direct != nil && nested != nil {
		return "", badRequest("Provide either `reasoning_effort` or `reasoning.effort`, not both.")
	}

	var raw any
	switch {
	case direct != nil:
		raw = direct
	case nested != nil:
		r, ok := nested.(map[string]any)
		if !ok || r["effort"] == nil {
			return "", badRequest("`reasoning` must be an object with an `effort` field.")
		}
		raw = r["effort"]
	default:
		return "", nil
	}

	effort, err := requireString(raw, "reasoning_effort")
	if err != nil {
		return "", err
	}
	if !supportedReasoningEfforts[effort] {
		return "", badRequest("`reasoning_effort` must be one of: high, low, medium.")
	}
	return effort, nil
}

func checkUnsupported(body map[string]any, fields []string) error {
	var bad []string
	for _, f := range fields {
		if body[f] != nil {
			bad = append(bad, f)
		}
	}
	if len(bad) > 0 {
		sort.Strings(bad)
		return badRequest(fmt.Sprintf("Unsupported request fields for this wrapper: %s.", strings.Join(bad, ", ")))
	}
	return nil
}

func validateChatRequest(body map[string]any) error {
	if v, _ := body["stream"].(bool); v {
		return badRequest("Unsupported request fields for this wrapper: stream.")
	}
	if n := body["n"]; n != nil {
		if nf, ok := n.(float64); !ok || nf != 1 {
			return badRequest("Unsupported request fields for this wrapper: n.")
		}
	}
	return checkUnsupported(body, []string{
		"tools", "tool_choice", "parallel_tool_calls",
		"temperature", "top_p", "max_tokens", "max_completion_tokens",
		"presence_penalty", "frequency_penalty", "logprobs", "top_logprobs",
	})
}

func validateResponsesRequest(body map[string]any) error {
	return checkUnsupported(body, []string{
		"stream", "tools", "tool_choice", "parallel_tool_calls",
		"previous_response_id", "conversation", "truncation",
		"temperature", "top_p", "max_output_tokens", "max_tool_calls",
		"prompt", "include",
	})
}

// ── message building ──────────────────────────────────────────────────────────

func normalizeContent(role string, content any) (string, error) {
	switch v := content.(type) {
	case string:
		return v, nil
	case []any:
		var parts []string
		for i, item := range v {
			p, ok := item.(map[string]any)
			if !ok {
				return "", badRequest(fmt.Sprintf("`messages[].content[%d]` must be an object.", i))
			}
			pt, _ := p["type"].(string)
			if !textPartTypes[pt] {
				return "", badRequest(fmt.Sprintf("Unsupported content part type `%s` in role `%s`.", pt, role))
			}
			t, ok := p["text"].(string)
			if !ok {
				return "", badRequest(fmt.Sprintf("`messages[].content[%d].text` must be a string.", i))
			}
			parts = append(parts, t)
		}
		return strings.Join(parts, "\n"), nil
	default:
		return "", badRequest(fmt.Sprintf("`messages[].content` for role `%s` must be a string or list of text parts.", role))
	}
}

func renderMessage(role, content string, name any) string {
	header := strings.ToUpper(role)
	if n, ok := name.(string); ok && n != "" {
		header = fmt.Sprintf("%s (%s)", header, n)
	}
	return strings.TrimSpace(header + ":\n" + content)
}

func buildPrompt(messages []any, structuredOutput bool) (string, error) {
	if len(messages) == 0 {
		return "", badRequest("`messages` must be a non-empty array.")
	}

	var system, transcript []string

	for i, raw := range messages {
		msg, ok := raw.(map[string]any)
		if !ok {
			return "", badRequest(fmt.Sprintf("`messages[%d]` must be an object.", i))
		}
		role, _ := msg["role"].(string)
		switch role {
		case "system", "developer", "user", "assistant":
		default:
			return "", badRequest(fmt.Sprintf("Unsupported message role `%s` at `messages[%d]`.", role, i))
		}
		if msg["tool_calls"] != nil {
			return "", badRequest(fmt.Sprintf("`messages[%d].tool_calls` is not supported.", i))
		}
		if msg["name"] != nil {
			if _, ok := msg["name"].(string); !ok {
				return "", badRequest(fmt.Sprintf("`messages[%d].name` must be a string.", i))
			}
		}
		content, err := normalizeContent(role, msg["content"])
		if err != nil {
			return "", err
		}
		rendered := renderMessage(role, content, msg["name"])
		if role == "system" || role == "developer" {
			system = append(system, rendered)
		} else {
			transcript = append(transcript, rendered)
		}
	}

	if len(transcript) == 0 {
		return "", badRequest("At least one `user` or `assistant` message is required.")
	}

	preamble := strings.Join([]string{
		"You are answering a chat.completions-compatible request routed through Codex exec.",
		"Produce exactly one assistant reply for the current conversation.",
		"Do not include role labels, XML tags, or wrapper commentary in the final answer.",
	}, "\n")
	if structuredOutput {
		preamble += "\nStructured output is enforced separately with JSON Schema. Return only the schema-matching final answer."
	}

	sections := []string{preamble}
	if len(system) > 0 {
		sections = append(sections, "<SYSTEM_MESSAGES>\n"+strings.Join(system, "\n\n")+"\n</SYSTEM_MESSAGES>")
	}
	sections = append(sections, "<CONVERSATION>\n"+strings.Join(transcript, "\n\n")+"\n</CONVERSATION>")
	return strings.Join(sections, "\n\n"), nil
}

// ── schema extraction ─────────────────────────────────────────────────────────

func extractSchema(rfRaw any, prefix string) (map[string]any, error) {
	if rfRaw == nil {
		return nil, nil
	}
	rf, ok := rfRaw.(map[string]any)
	if !ok {
		return nil, badRequest(fmt.Sprintf("`%s` must be an object.", prefix))
	}
	rfType, _ := rf["type"].(string)
	if rfType == "" || rfType == "text" {
		return nil, nil
	}
	if rfType != "json_schema" {
		return nil, badRequest(fmt.Sprintf("Only `%s.type = \"json_schema\"` is supported.", prefix))
	}
	jsc, ok := rf["json_schema"].(map[string]any)
	if !ok {
		return nil, badRequest(fmt.Sprintf("`%s.json_schema` must be an object.", prefix))
	}
	schema, ok := jsc["schema"].(map[string]any)
	if !ok {
		return nil, badRequest(fmt.Sprintf("`%s.json_schema.schema` must be an object.", prefix))
	}
	return schema, nil
}

func extractChatSchema(body map[string]any) (map[string]any, error) {
	return extractSchema(body["response_format"], "response_format")
}

func extractResponsesSchema(body map[string]any) (map[string]any, error) {
	tc, ok := body["text"].(map[string]any)
	if !ok {
		if body["text"] != nil {
			return nil, badRequest("`text` must be an object.")
		}
		return nil, nil
	}
	return extractSchema(tc["format"], "text.format")
}

// ── responses input ───────────────────────────────────────────────────────────

func coerceInputItem(item any, i int) (map[string]any, error) {
	m, ok := item.(map[string]any)
	if !ok {
		return nil, badRequest(fmt.Sprintf("`input[%d]` must be an object.", i))
	}
	itemType, _ := m["type"].(string)
	if itemType == "" || itemType == "message" {
		if m["role"] == nil || m["content"] == nil {
			return nil, badRequest(fmt.Sprintf("`input[%d]` must have `role` and `content`.", i))
		}
		out := map[string]any{"role": m["role"], "content": m["content"]}
		if m["name"] != nil {
			out["name"] = m["name"]
		}
		if m["tool_calls"] != nil {
			out["tool_calls"] = m["tool_calls"]
		}
		return out, nil
	}
	if textPartTypes[itemType] {
		t, ok := m["text"].(string)
		if !ok {
			return nil, badRequest(fmt.Sprintf("`input[%d].text` must be a string.", i))
		}
		return map[string]any{"role": "user", "content": t}, nil
	}
	return nil, badRequest(fmt.Sprintf("Unsupported `input[%d].type` value `%s`.", i, itemType))
}

func buildResponsesMessages(body map[string]any) ([]any, error) {
	var msgs []any
	if instr := body["instructions"]; instr != nil {
		s, err := requireString(instr, "instructions")
		if err != nil {
			return nil, err
		}
		msgs = append(msgs, map[string]any{"role": "system", "content": s})
	}
	raw := body["input"]
	if raw == nil {
		return nil, badRequest("`input` is required for `/v1/responses`.")
	}
	if s, ok := raw.(string); ok {
		msgs = append(msgs, map[string]any{"role": "user", "content": s})
		return msgs, nil
	}
	list, ok := raw.([]any)
	if !ok || len(list) == 0 {
		return nil, badRequest("`input` must be a non-empty string or array.")
	}
	for i, item := range list {
		msg, err := coerceInputItem(item, i)
		if err != nil {
			return nil, err
		}
		msgs = append(msgs, msg)
	}
	return msgs, nil
}

// ── models discovery ──────────────────────────────────────────────────────────

func loadModels(configPath string) []string {
	var models []string
	seen := map[string]bool{}
	add := func(m string) {
		m = strings.TrimSpace(m)
		if m != "" && !seen[m] {
			seen[m] = true
			models = append(models, m)
		}
	}

	if env := os.Getenv("CODEX_CHAT_WRAPPER_MODELS"); env != "" {
		for _, m := range strings.Split(env, ",") {
			add(m)
		}
	}

	data, err := os.ReadFile(configPath)
	if err == nil {
		for _, match := range modelLineRe.FindAllSubmatch(data, -1) {
			add(string(match[1]))
		}
	}
	return models
}

// ── codex exec ────────────────────────────────────────────────────────────────

func runCodex(ctx context.Context, prompt, model, preset, effort string, schema map[string]any, cwd string) (string, error) {
	tmpDir, err := os.MkdirTemp("", "codex2api-")
	if err != nil {
		return "", serverError("failed to create temp dir: "+err.Error(), "server_error")
	}
	defer os.RemoveAll(tmpDir)

	outputPath := filepath.Join(tmpDir, "response.txt")

	args := []string{
		"exec", "--ephemeral", "--skip-git-repo-check",
		"--color", "never",
		"-m", model,
		"-o", outputPath,
	}
	if preset != "" {
		args = append(args, "-p", preset)
	}
	if effort != "" {
		args = append(args, "-c", fmt.Sprintf(`model_reasoning_effort="%s"`, effort))
	}
	if schema != nil {
		schemaPath := filepath.Join(tmpDir, "schema.json")
		b, _ := json.Marshal(schema)
		if err := os.WriteFile(schemaPath, b, 0600); err != nil {
			return "", serverError("failed to write schema: "+err.Error(), "server_error")
		}
		args = append(args, "--output-schema", schemaPath)
	}
	args = append(args, "-")

	cmd := exec.CommandContext(ctx, "codex", args...)
	cmd.Dir = cwd
	cmd.Stdin = strings.NewReader(prompt)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", timeoutError("codex exec timed out.")
		}
		details := strings.TrimSpace(stderr.String())
		if details == "" {
			details = strings.TrimSpace(stdout.String())
		}
		if details == "" {
			details = "codex exec failed without output."
		}
		return "", serverError("codex exec failed: "+details, "codex_exec_failed")
	}

	result, err := os.ReadFile(outputPath)
	if err != nil {
		return "", serverError("codex exec completed without writing the output file.", "missing_output")
	}
	return string(result), nil
}

// ── HTTP handlers ─────────────────────────────────────────────────────────────

func (s *srv) healthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *srv) listModels(w http.ResponseWriter, r *http.Request) {
	models := loadModels(s.configPath)
	data := make([]any, len(models))
	for i, m := range models {
		data[i] = map[string]any{"id": m, "object": "model", "created": 0, "owned_by": "codex2api"}
	}
	writeJSON(w, 200, map[string]any{"object": "list", "data": data})
}

func (s *srv) chatCompletions(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, badRequest("Request body must be valid JSON."))
		return
	}

	if err := validateChatRequest(body); err != nil {
		writeError(w, err.(*reqErr))
		return
	}

	requestedModel := body["model"]
	model, preset, err := parseModelAndPreset(requestedModel)
	if err != nil {
		writeError(w, err.(*reqErr))
		return
	}
	effort, err := parseReasoningEffort(body)
	if err != nil {
		writeError(w, err.(*reqErr))
		return
	}
	schema, err := extractChatSchema(body)
	if err != nil {
		writeError(w, err.(*reqErr))
		return
	}

	msgs, ok := body["messages"].([]any)
	if !ok {
		writeError(w, badRequest("`messages` must be a non-empty array."))
		return
	}
	prompt, err := buildPrompt(msgs, schema != nil)
	if err != nil {
		writeError(w, err.(*reqErr))
		return
	}

	s.sem <- struct{}{}
	defer func() { <-s.sem }()

	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(s.timeoutSeconds)*time.Second)
	defer cancel()

	text, err := runCodex(ctx, prompt, model, preset, effort, schema, s.cwd)
	if err != nil {
		writeError(w, err.(*reqErr))
		return
	}

	writeJSON(w, 200, map[string]any{
		"id":      "chatcmpl-" + randHex(),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   requestedModel,
		"choices": []any{map[string]any{
			"index":         0,
			"message":       map[string]any{"role": "assistant", "content": text},
			"finish_reason": "stop",
		}},
	})
}

func (s *srv) responses(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, badRequest("Request body must be valid JSON."))
		return
	}

	if err := validateResponsesRequest(body); err != nil {
		writeError(w, err.(*reqErr))
		return
	}

	requestedModel := body["model"]
	model, preset, err := parseModelAndPreset(requestedModel)
	if err != nil {
		writeError(w, err.(*reqErr))
		return
	}
	effort, err := parseReasoningEffort(body)
	if err != nil {
		writeError(w, err.(*reqErr))
		return
	}
	schema, err := extractResponsesSchema(body)
	if err != nil {
		writeError(w, err.(*reqErr))
		return
	}
	msgs, err := buildResponsesMessages(body)
	if err != nil {
		writeError(w, err.(*reqErr))
		return
	}
	prompt, err := buildPrompt(msgs, schema != nil)
	if err != nil {
		writeError(w, err.(*reqErr))
		return
	}

	s.sem <- struct{}{}
	defer func() { <-s.sem }()

	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(s.timeoutSeconds)*time.Second)
	defer cancel()

	text, err := runCodex(ctx, prompt, model, preset, effort, schema, s.cwd)
	if err != nil {
		writeError(w, err.(*reqErr))
		return
	}

	writeJSON(w, 200, map[string]any{
		"id":                 "resp_" + randHex(),
		"object":             "response",
		"created_at":         time.Now().Unix(),
		"status":             "completed",
		"error":              nil,
		"incomplete_details": nil,
		"instructions":       body["instructions"],
		"model":              requestedModel,
		"output": []any{map[string]any{
			"id":     "msg_" + randHex(),
			"type":   "message",
			"status": "completed",
			"role":   "assistant",
			"content": []any{map[string]any{
				"type":        "output_text",
				"text":        text,
				"annotations": []any{},
			}},
		}},
	})
}

// ── CLI & main ────────────────────────────────────────────────────────────────

func envStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

func main() {
	host := flag.String("host", envStr("CODEX_CHAT_WRAPPER_HOST", defaultHost), "Listen address")
	port := flag.Int("port", envInt("CODEX_CHAT_WRAPPER_PORT", defaultPort), "Listen port")
	cwd := flag.String("cwd", envStr("CODEX_CHAT_WRAPPER_CWD", mustGetwd()), "Working directory for codex exec")
	configPath := flag.String("codex-config",
		expandHome(envStr("CODEX_CHAT_WRAPPER_CODEX_CONFIG", "~/.codex/config.toml")),
		"Path to ~/.codex/config.toml for model discovery")
	maxConcurrency := flag.Int("max-concurrency",
		envInt("CODEX_CHAT_WRAPPER_MAX_CONCURRENCY", defaultMaxConcurrency),
		fmt.Sprintf("Max concurrent codex exec processes (default %d)", defaultMaxConcurrency))
	timeoutSeconds := flag.Int("timeout-seconds",
		envInt("CODEX_CHAT_WRAPPER_TIMEOUT_SECONDS", defaultTimeoutSeconds),
		fmt.Sprintf("Per-request timeout in seconds (default %d)", defaultTimeoutSeconds))
	flag.Parse()

	if *maxConcurrency < 1 {
		log.Fatal("--max-concurrency must be >= 1")
	}
	if *timeoutSeconds < 1 {
		log.Fatal("--timeout-seconds must be >= 1")
	}

	s := &srv{
		cwd:            *cwd,
		configPath:     *configPath,
		timeoutSeconds: *timeoutSeconds,
		sem:            make(chan struct{}, *maxConcurrency),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.healthz)
	mux.HandleFunc("GET /v1/models", s.listModels)
	mux.HandleFunc("POST /v1/chat/completions", s.chatCompletions)
	mux.HandleFunc("POST /v1/responses", s.responses)

	addr := fmt.Sprintf("%s:%d", *host, *port)
	log.Printf("codex2api listening on http://%s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}

func mustGetwd() string {
	d, err := os.Getwd()
	if err != nil {
		return "."
	}
	return d
}
