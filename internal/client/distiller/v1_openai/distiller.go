package v1openai

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/tmc/langchaingo/embeddings"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
	v1 "github.com/w-h-a/gomento/api/domain/v1"
	"github.com/w-h-a/gomento/internal/client/distiller"
)

type v1OpenaiDistiller struct {
	options  distiller.Options
	llm      llms.Model
	embedder embeddings.Embedder
}

func (d *v1OpenaiDistiller) Distill(ctx context.Context, history []v1.Message) (*v1.Skill, error) {
	slog.InfoContext(ctx, "distilling session", "message_count", len(history))

	content := []llms.MessageContent{
		llms.TextParts(llms.ChatMessageTypeSystem, d.systemPrompt()),
		llms.TextParts(llms.ChatMessageTypeHuman, d.userPrompt(history)),
	}

	rsp, err := d.llm.GenerateContent(
		ctx,
		content,
		llms.WithJSONMode(),
	)
	if err != nil {
		return nil, fmt.Errorf("llm generation failed: %w", err)
	}

	raw := rsp.Choices[0].Content

	slog.InfoContext(ctx, "llm response received", "response_length", len(raw))

	var result v1.Skill
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return nil, fmt.Errorf("failed to parse llm json: %w", err)
	}

	if len(result.SOP) == 0 || len(result.Trigger) == 0 {
		return nil, fmt.Errorf("llm returned incomplete data: %s", raw)
	}

	slog.InfoContext(ctx, "generating embedding", "trigger", result.Trigger)

	vectors, err := d.embedder.EmbedDocuments(ctx, []string{result.Trigger})
	if err != nil {
		return nil, fmt.Errorf("embedding generation failed: %w", err)
	}

	if len(vectors) == 0 || len(vectors[0]) == 0 {
		return nil, fmt.Errorf("embedding returned empty vector")
	}

	result.Embedding = vectors[0]

	return &result, nil
}

func (d *v1OpenaiDistiller) userPrompt(history []v1.Message) string {
	var sb strings.Builder

	sb.WriteString("Analyze the following conversation and extract the core automation skill:\n\n")

	for _, msg := range history {
		for _, part := range msg.Parts {
			line := d.packMessageLine(msg.Role, part)
			sb.WriteString(line + "\n")
		}
	}

	return sb.String()
}

func (d *v1OpenaiDistiller) systemPrompt() string {
	return `You are an expert at distilling user intents into automation "Skills".
A Skill consists of two parts:
1. Trigger: A concise phrase describing WHEN this should happen.
2. SOP: A detailed description of WHAT should happen (Standard Operating Procedure).

Output MUST be a valid JSON object with keys "trigger" and "sop".
Example: {"trigger": "every morning at 9am", "sop": "check jira for high priority bugs"}`
}

func (d *v1OpenaiDistiller) packMessageLine(role string, part v1.Part) string {
	switch role {
	case "assistant":
		role = "agent"
	case "tool", "function":
		role = "agent_action"
	}

	switch part.Type {
	case "text":
		return fmt.Sprintf("<%s> %s", role, part.Text)
	case "file":
		name := "unknown_file"
		if part.Meta != nil {
			if n, ok := part.Meta["filename"].(string); ok {
				name = n
			}
		}
		return fmt.Sprintf("<%s> [file: %s]", role, name)
	case "tool-call":
		funcName := "unknown_tool"
		params := "{}"

		if part.Meta != nil {
			if n, ok := part.Meta["tool_name"].(string); ok {
				funcName = n
			} else if n, ok := part.Meta["function_name"].(string); ok {
				funcName = n
			}

			if p, ok := part.Meta["arguments"]; ok {
				params = fmt.Sprintf("%v", p)
			} else if p, ok := part.Meta["parameters"]; ok {
				params = fmt.Sprintf("%v", p)
			}
		}

		return fmt.Sprintf("<%s> USE TOOL %s, WITH PARAMS %s", role, funcName, params)
	case "tool-result":
		return fmt.Sprintf("<%s> TOOL RESULT: %s", role, part.Text)
	default:
		return fmt.Sprintf("<%s> [%s]", role, part.Type)
	}
}

func NewV1Distiller(opts ...distiller.Option) distiller.V1Distiller {
	options := distiller.NewOptions(opts...)

	// TODO: validate options

	d := &v1OpenaiDistiller{
		options: options,
	}

	llmOpts := []openai.Option{
		openai.WithToken(options.ApiKey),
		openai.WithModel(options.Model),
	}

	llm, err := openai.New(llmOpts...)
	if err != nil {
		detail := "failed to initialize model for v1 openai distiller"
		slog.ErrorContext(context.Background(), detail, "error", err)
		panic(detail)
	}

	d.llm = llm

	emb, err := embeddings.NewEmbedder(llm)
	if err != nil {
		detail := "failed to initialize embedder for v1 openai distiller"
		slog.ErrorContext(context.Background(), detail, "error", err)
		panic(detail)
	}

	d.embedder = emb

	return d
}
