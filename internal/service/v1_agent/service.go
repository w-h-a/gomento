package v1agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/tmc/langchaingo/llms"
	v1 "github.com/w-h-a/gomento/api/domain/v1"
	toolprovider "github.com/w-h-a/gomento/internal/client/tool_provider"
	"github.com/w-h-a/gomento/internal/service"
	"github.com/w-h-a/gomento/internal/util"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type V1Agent struct {
	*service.Service
	model        llms.Model
	toolProvider toolprovider.V1ToolProvider
	tracer       trace.Tracer
	instructions string
}

func (a *V1Agent) CreateSession(ctx context.Context, spaceId *uuid.UUID) (uuid.UUID, error) {
	ctx, span := a.tracer.Start(ctx, "agent.CreateSession")
	defer span.End()

	if spaceId != nil {
		span.SetAttributes(attribute.String("space_id", spaceId.String()))
	}

	args := map[string]any{}
	if spaceId != nil {
		args["space_id"] = spaceId.String()
	}

	rspStr, err := a.toolProvider.Call(ctx, "create_session", args)
	if err != nil {
		span.RecordError(err)
		return uuid.Nil, fmt.Errorf("failed to create session: %w", err)
	}

	var sess v1.Session
	if err := json.Unmarshal([]byte(rspStr), &sess); err != nil {
		span.RecordError(err)
		return uuid.Nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return sess.Id, nil
}

func (a *V1Agent) TakeTurns(ctx context.Context, sessionId uuid.UUID, input string) (string, []string, error) {
	ctx, span := a.tracer.Start(ctx, "agent.TakeTurns")
	defer span.End()

	span.SetAttributes(attribute.String("session_id", sessionId.String()))

	availableTools, err := a.toolProvider.List(ctx)
	if err != nil {
		span.RecordError(err)
		return "", nil, fmt.Errorf("failed to list tools: %w", err)
	}

	var lcTools []llms.Tool
	for _, t := range availableTools {
		params := map[string]any{
			"type":       t.Schema.Type,
			"properties": t.Schema.Properties,
		}
		if len(t.Schema.Required) > 0 {
			params["required"] = t.Schema.Required
		}

		lcTools = append(lcTools, llms.Tool{
			Type: "function",
			Function: &llms.FunctionDefinition{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  params,
			},
		})
	}

	sessionContext := fmt.Sprintf("Current Session ID: %s\n", sessionId.String())
	fullInstructions := sessionContext + a.instructions

	history := []llms.MessageContent{
		llms.TextParts(llms.ChatMessageTypeSystem, fullInstructions),
		llms.TextParts(llms.ChatMessageTypeHuman, input),
	}

	var toolCallsLog []string

	maxTurns := 10

	for i := range maxTurns {
		span.AddEvent("taking_turn", trace.WithAttributes(
			attribute.Int("iteration", i+1),
		))

		// Ask LLM what to do
		rsp, err := a.model.GenerateContent(ctx, history, llms.WithTools(lcTools))
		if err != nil {
			span.RecordError(err)
			return "", nil, fmt.Errorf("llm generation failed: %w", err)
		}

		if len(rsp.Choices) == 0 {
			err := fmt.Errorf("llm returned no choices")
			span.RecordError(err)
			return "", nil, err
		}

		var result strings.Builder

		choice := rsp.Choices[0]

		span.SetAttributes(
			attribute.String("llm.finish_reason", choice.StopReason),
			attribute.Int("llm.completion_tokens", util.GetSafeInt(choice.GenerationInfo, "CompletionTokens")),
			attribute.Int("llm.prompt_tokens", util.GetSafeInt(choice.GenerationInfo, "PromptTokens")),
			attribute.Int("llm.total_tokens", util.GetSafeInt(choice.GenerationInfo, "TotalTokens")),
		)

		if len(choice.Content) > 0 {
			span.SetAttributes(attribute.String("ai.decision.reasoning", choice.Content))
		}

		var toolNames []string
		var toolArgs []string

		for _, tc := range choice.ToolCalls {
			toolNames = append(toolNames, tc.FunctionCall.Name)
			toolArgs = append(toolArgs, tc.FunctionCall.Arguments)
		}

		if len(toolNames) > 0 {
			span.SetAttributes(
				attribute.StringSlice("ai.decision.tools", toolNames),
				attribute.StringSlice("ai.decision.args", toolArgs),
			)
		}

		if len(choice.Content) > 0 {
			result.WriteString(choice.Content + "\n")
		}

		// If no tool calls, bail early
		if len(choice.ToolCalls) == 0 {
			span.AddEvent("finishing_turns")
			return result.String(), toolCallsLog, nil
		}

		// Append the LLM's response to history
		var parts []llms.ContentPart
		for _, tc := range choice.ToolCalls {
			parts = append(parts, llms.ToolCall{ID: tc.ID, Type: tc.Type, FunctionCall: tc.FunctionCall})
		}
		history = append(history, llms.MessageContent{
			Role:  llms.ChatMessageTypeAI,
			Parts: parts,
		})

		// Execute the tool call
		for _, tc := range choice.ToolCalls {
			fnName := tc.FunctionCall.Name
			fnArgsStr := tc.FunctionCall.Arguments

			toolCallsLog = append(toolCallsLog, fnName)

			var args map[string]any
			var toolResult string

			if err := json.Unmarshal([]byte(fnArgsStr), &args); err != nil {
				toolResult = fmt.Sprintf("Error: failed to parse arguments JSON: %v", err)
			} else {
				toolResult, err = a.toolProvider.Call(ctx, fnName, args)
				if err != nil {
					toolResult = fmt.Sprintf("Error: %v", err)
				}
			}

			// Feed the result back to the LLM
			history = append(history, llms.MessageContent{
				Role: llms.ChatMessageTypeTool,
				Parts: []llms.ContentPart{
					llms.ToolCallResponse{
						ToolCallID: tc.ID,
						Name:       fnName,
						Content:    toolResult,
					},
				},
			})
		}

	}

	span.AddEvent("max_turns_exceeded")

	return "Error: max turns exceeded", toolCallsLog, nil
}

func New(model llms.Model, toolProvider toolprovider.V1ToolProvider, instructions string) *V1Agent {
	s := service.New()
	return &V1Agent{
		Service:      s,
		model:        model,
		toolProvider: toolProvider,
		tracer:       otel.Tracer("github.com/w-h-a/gomento/internal/service/agent"),
		instructions: instructions,
	}
}
