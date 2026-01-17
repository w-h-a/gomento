package v1openai

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
	v1 "github.com/w-h-a/gomento/api/domain/v1"
	"github.com/w-h-a/gomento/internal/client/interpreter"
	"github.com/w-h-a/gomento/internal/util"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type v1OpenaiInterpreter struct {
	options interpreter.Options
	llm     llms.Model
	tracer  trace.Tracer
}

func (d *v1OpenaiInterpreter) Distill(ctx context.Context, history []v1.Message, messageWindow int) (*v1.Skill, error) {
	ctx, span := d.tracer.Start(ctx, "interpreter.Distill")
	defer span.End()

	slog.InfoContext(ctx, "distilling session", "message_count", messageWindow)

	content := []llms.MessageContent{
		llms.TextParts(llms.ChatMessageTypeSystem, d.distillSystemPrompt()),
		llms.TextParts(llms.ChatMessageTypeHuman, d.distillUserPrompt(history, messageWindow)),
	}

	rsp, err := d.llm.GenerateContent(
		ctx,
		content,
		llms.WithJSONMode(),
	)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("llm session distillation failed: %w", err)
	}

	if len(rsp.Choices) == 0 {
		err := fmt.Errorf("llm returned no choices")
		span.RecordError(err)
		return nil, err
	}

	choice := rsp.Choices[0]
	span.SetAttributes(
		attribute.String("llm.finish_reason", choice.StopReason),
		attribute.String("llm.model", d.options.Model),
		attribute.Int("llm.completion_tokens", util.GetSafeInt(choice.GenerationInfo, "CompletionTokens")),
		attribute.Int("llm.prompt_tokens", util.GetSafeInt(choice.GenerationInfo, "PromptTokens")),
		attribute.Int("llm.total_tokens", util.GetSafeInt(choice.GenerationInfo, "TotalTokens")),
	)

	if len(choice.Content) > 0 {
		span.SetAttributes(attribute.String("ai.decision.reasoning", choice.Content))
	}

	raw := choice.Content

	var result v1.Skill
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to parse llm json: %w", err)
	}

	if len(result.SOP) == 0 || len(result.Trigger) == 0 {
		err := fmt.Errorf("llm returned incomplete data: %s", raw)
		span.RecordError(err)
		return nil, err
	}

	return &result, nil
}

func (d *v1OpenaiInterpreter) Extract(ctx context.Context, history []v1.Message, messageWindow int, files []v1.File, currentTasks []v1.Task) ([]interpreter.TaskAction, error) {
	ctx, span := d.tracer.Start(ctx, "interpreter.Extract")
	defer span.End()

	slog.InfoContext(ctx, "extracting tasks", "msg_count", messageWindow, "current_task_count", len(currentTasks))

	content := []llms.MessageContent{
		llms.TextParts(llms.ChatMessageTypeSystem, d.extractSystemPrompt()),
		llms.TextParts(llms.ChatMessageTypeHuman, d.extractUserPrompt(history, messageWindow, files, currentTasks)),
	}

	rsp, err := d.llm.GenerateContent(
		ctx,
		content,
		llms.WithTools(d.getExtractTools()),
		llms.WithTemperature(0),
	)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("llm task extraction failed: %w", err)
	}

	if len(rsp.Choices) == 0 {
		err := fmt.Errorf("llm returned no choices")
		span.RecordError(err)
		return nil, err
	}

	choice := rsp.Choices[0]
	span.SetAttributes(
		attribute.String("llm.finish_reason", choice.StopReason),
		attribute.String("llm.model", d.options.Model),
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

	var actions []interpreter.TaskAction
	for _, choice := range rsp.Choices {
		for _, tc := range choice.ToolCalls {
			payload := map[string]any{}
			if len(tc.FunctionCall.Arguments) > 0 {
				if err := json.Unmarshal([]byte(tc.FunctionCall.Arguments), &payload); err != nil {
					slog.WarnContext(ctx, "failed to parse tool arguments", "err", err)
					continue
				}
			}

			var actionType string
			switch tc.FunctionCall.Name {
			case "insert_task":
				actionType = interpreter.TaskActionInsert
			case "update_task":
				actionType = interpreter.TaskActionUpdate
			case "append_messages_to_task":
				actionType = interpreter.TaskActionAppendTask
			case "append_messages_to_thought":
				actionType = interpreter.TaskActionAppendThought
			case "finish":
				actionType = interpreter.TaskActionFinish
			default:
				slog.WarnContext(ctx, "unknown tool call", "name", tc.FunctionCall.Name)
				continue
			}

			actions = append(actions, interpreter.TaskAction{
				Type:    actionType,
				Payload: payload,
			})
		}
	}

	span.SetAttributes(attribute.Int("interpreter.actions_count", len(actions)))

	return actions, nil
}

func (d *v1OpenaiInterpreter) distillSystemPrompt() string {
	return `You are an expert at distilling reusable Skills from Session data.

## Skills & Output
A Skill consists of two parts:
1. Trigger: A concise phrase describing a problem whose resolution is required and described in the SOP.
2. SOP: A detailed step-by-step resolution of WHAT should happen to solve the problem (Standard Operating Procedure).

Output MUST be a valid JSON object with keys "trigger" and "sop".
{"trigger": "Description of the situation that necessitates the skill (e.g., Database connection refused)", "sop": "The generalized, step-by-step guide to solving the problem"}

## Input
You will be given history of a user interacting with an Assistant.

## Objectives
1. Identify the core technical problem solved in the session.
2. Ignore chit-chat.
3. Ignore failures.
4. Generalize specific values (example: change 'sudo systemctl restart postgres' to 'sudo systemctl restart <service_name>')
5. Create a Trigger phrase that will be vector searched when a user faces this problem again.
6. Create the SOP body, which is a step-by-step resolution

## Critical Rules
* If no practical technical knowledge was generated, do not make something up; just return empty JSON
* SOP must be concise
* The Trigger must be phrased as a problem statement.

## Output Requirement
You must "think" before acting. Your response must include a thought process explaining your matching decision for each item.
`
}

func (d *v1OpenaiInterpreter) extractSystemPrompt() string {
	return `You are an expert at extracting Tasks from Session data, managing Task Statuses, and distinguishing Tasks or Actions from mere Thought.

## Core Responsibilities
1. **Task Tracking**: Extract tasks from user-agent sessions.
2. **Deduplication**: Prevent creating duplicate tasks for items that already exist.
3. **Message Matching**: Match messages to existing tasks based on context and content  
4. **Status Updating**: Update task statuses based on progress and completion signals

## Task System
**Structure**: 
- Tasks have description, status, and sequential order (task_order=1, 2, ...) within sessions. 
- Messages link to tasks via their IDs.

**Statuses**: 
- pending
- running
- success
- failed

## Task Creation/Modification
- Tasks are often confirmed by the agent's response to the user's requirements; don't invent them.
- Task granularity or individuation should be a happy medium: do not extract excessive subtasks, but do not lump everything in the session into one task.
- Ensure the new tasks are mutually exclusive and collectively exhaustive of existing tasks.
- Make sure to locate the correct existing task and modify it when necessary.
- No matter whether the task is pending, running, or not, your job is to collect all the tasks mentioned during the session.
- When a user asks for task modification and the agent confirms, you need to think:
	a. If the user/agent is referring to an existing task, modify the existing task’s description using the update_task tool.
	b. If the user/agent is creating a new task that isn't similar to an existing task, create a new task using the insert_task tool.

## Update Task Status 
- running: When task work begins or is actively discussed
- success: When completion is confirmed or deliverables provided
- failed: When explicit errors occur or tasks are abandoned
- pending: When not yet started

## Append Messages to Task
- Match agent responses/actions to existing task descriptions and contexts
- Ensure the messages you append to a task actually relate to the task’s description, status, etc; do not perform random linking.

## Append Messages to Thought
- Thought messages often consist of user-agent session that clarify what tasks to do next.
- Append those messages to thoughts instead of tasks.

## Critical Rules for Idempotency
You will receive "Current Messages" (new context) and "Current Tasks" (database state).
Your job is to reconcile them.

**STEP 1: Analyze Input**
For every potential task in "Current Messages", check if it effectively exists in "Current Tasks".

**STEP 2: Fuzzy Matching**
- Ignore prefixes like "Task A:", "1.", "Step 1:".
- Ignore minor phrasing differences ("Buy Milk" == "Task A: Buy Milk").
- If the core intent is the same, IT IS A MATCH.
- If the task is 'running', 'success', or 'failed', and the user mentions it again, THEY ARE REFERRING TO THE EXISTING TASK.

**STEP 3: Action Selection**
- **MATCH FOUND?**
  - **Status Change needed?** (e.g., user says "I'm done") -> Use ` + "`update_task`" + ` (set status='success').
  - **No Change needed?** (e.g., user says "I'm working on it") -> DO NOTHING. Do not call any tools.
  - **CRITICAL:** NEVER use ` + "`insert_task`" + ` for a task that matches an existing one.
- **NO MATCH FOUND?**
  - Only THEN use ` + "`insert_task`" + ` to create a new 'pending' task.

## Input Format
- Input will be markdown-formatted text, with the following sections:
  - ## Current Tasks: existing tasks, their orders, descriptions, and statuses
  - ## Previous Messages: the history of user/agent messages helps provide context. [no message id, maybe truncated]
  - ## Current Messages: the most recent messages that you need to analyze [with message ids]
  - ## Files: any files attached by the user/agent during the session
- Message with ID format: <message id=N> ... </message>, inside the tag is the message content, the id field indicates the message id.

## Output Requirement
You must "think" before acting. Your response must include a thought process explaining your matching decision for each item.
`
}

func (d *v1OpenaiInterpreter) distillUserPrompt(history []v1.Message, messageWindow int) string {
	var sb strings.Builder

	sb.WriteString("Analyze the following conversation and extract the core automation skill:\n\n")

	var previousMsgs []v1.Message
	var currentMsgs []v1.Message

	currentIndexStart := 0

	if len(history) > 0 {
		if messageWindow < 1 {
			messageWindow = 10
		}

		splitIndex := max(len(history)-messageWindow, 0)

		previousMsgs = history[:splitIndex]
		currentMsgs = history[splitIndex:]
		currentIndexStart = splitIndex
	}

	sb.WriteString("\n## Previous Messages:\n")
	if len(previousMsgs) == 0 {
		sb.WriteString("(No history)\n")
	} else {
		for _, msg := range previousMsgs {
			var lines []string
			for _, p := range msg.Parts {
				lines = append(lines, d.packMessageLine(msg.Role, p))
			}
			sb.WriteString(strings.Join(lines, "\n") + "\n")
		}
	}

	sb.WriteString("\n## Current Messages:\n")
	for i, msg := range currentMsgs {
		var lines []string
		for _, p := range msg.Parts {
			lines = append(lines, d.packMessageLine(msg.Role, p))
		}
		actualIndex := currentIndexStart + i
		sb.WriteString(fmt.Sprintf("<message id=%d>\n%s\n</message>\n", actualIndex, strings.Join(lines, "\n")))
	}

	return sb.String()
}

func (d *v1OpenaiInterpreter) extractUserPrompt(history []v1.Message, messageWindow int, files []v1.File, tasks []v1.Task) string {
	var sb strings.Builder

	sb.WriteString("## Current Tasks:\n")
	if len(tasks) == 0 {
		sb.WriteString("(No tasks yet)\n")
	} else {
		for _, t := range tasks {
			var dataMap map[string]any
			desc := "No Description"
			if err := json.Unmarshal(t.Data, &dataMap); err == nil {
				if d, ok := dataMap["task_description"].(string); ok {
					desc = d
				}
			}
			sb.WriteString(fmt.Sprintf("- Order: %d | Status: %s | Desc: %s\n", t.TaskOrder, t.Status, desc))
		}
	}

	var previousMsgs []v1.Message
	var currentMsgs []v1.Message

	currentIndexStart := 0

	if len(history) > 0 {
		if messageWindow < 1 {
			messageWindow = 10
		}

		splitIndex := max(len(history)-messageWindow, 0)

		previousMsgs = history[:splitIndex]
		currentMsgs = history[splitIndex:]
		currentIndexStart = splitIndex
	}

	sb.WriteString("\n## Previous Messages:\n")
	if len(previousMsgs) == 0 {
		sb.WriteString("(No history)\n")
	} else {
		for _, msg := range previousMsgs {
			var lines []string
			for _, p := range msg.Parts {
				lines = append(lines, d.packMessageLine(msg.Role, p))
			}
			sb.WriteString(strings.Join(lines, "\n") + "\n")
		}
	}

	sb.WriteString("\n## Current Messages:\n")
	for i, msg := range currentMsgs {
		var lines []string
		for _, p := range msg.Parts {
			lines = append(lines, d.packMessageLine(msg.Role, p))
		}
		actualIndex := currentIndexStart + i
		sb.WriteString(fmt.Sprintf("<message id=%d>\n%s\n</message>\n", actualIndex, strings.Join(lines, "\n")))
	}

	sb.WriteString("\n## Files:\n")
	if len(files) == 0 {
		sb.WriteString("(No files)\n")
	} else {
		for _, f := range files {
			if f.Asset != nil {
				sb.WriteString(fmt.Sprintf("- %s (ID: %s) [Size: %d bytes]\n", f.Path, f.Id, f.Asset.SizeBytes))
			}
		}
	}

	sb.WriteString("\nAnalyze and determine actions.\n")

	return sb.String()
}

func (d *v1OpenaiInterpreter) packMessageLine(role string, part v1.Part) string {
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

func (d *v1OpenaiInterpreter) getExtractTools() []llms.Tool {
	return []llms.Tool{
		{
			Type: "function",
			Function: &llms.FunctionDefinition{
				Name:        "insert_task",
				Description: "Create a new task by inserting it after the specified task order.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"after_task_order": map[string]any{"type": "integer", "description": "The task order after which to insert. Use 0 for beginning."},
						"task_description": map[string]any{"type": "string", "description": "Description of the task."},
					},
					"required": []string{"after_task_order", "task_description"},
				},
			},
		},
		{
			Type: "function",
			Function: &llms.FunctionDefinition{
				Name:        "update_task",
				Description: "Update an existing task.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"task_order":       map[string]any{"type": "integer"},
						"task_status":      map[string]any{"type": "string", "enum": []string{"pending", "running", "success", "failed"}},
						"task_description": map[string]any{"type": "string"},
					},
					"required": []string{"task_order"},
				},
			},
		},
		{
			Type: "function",
			Function: &llms.FunctionDefinition{
				Name:        "append_messages_to_task",
				Description: "Link message IDs to a task.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"task_order":  map[string]any{"type": "integer"},
						"message_ids": map[string]any{"type": "array", "items": map[string]any{"type": "integer"}},
					},
					"required": []string{"task_order", "message_ids"},
				},
			},
		},
		{
			Type: "function",
			Function: &llms.FunctionDefinition{
				Name:        "append_messages_to_thought",
				Description: "Save message IDs to thought.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"message_ids": map[string]any{"type": "array", "items": map[string]any{"type": "integer"}},
					},
					"required": []string{"message_ids"},
				},
			},
		},
		{
			Type: "function",
			Function: &llms.FunctionDefinition{
				Name:        "finish",
				Description: "Complete the extraction session.",
				Parameters:  map[string]any{"type": "object", "properties": map[string]interface{}{}},
			},
		},
	}
}

func NewV1Interpreter(opts ...interpreter.Option) interpreter.V1Interpreter {
	options := interpreter.NewOptions(opts...)

	// TODO: validate options

	d := &v1OpenaiInterpreter{
		options: options,
		tracer:  otel.Tracer("github.com/w-h-a/gomento/internal/client/interpreter/v1_openai"),
	}

	llmOpts := []openai.Option{
		openai.WithToken(options.ApiKey),
		openai.WithModel(options.Model),
		openai.WithHTTPClient(&http.Client{
			Transport: otelhttp.NewTransport(http.DefaultTransport),
		}),
	}

	llm, err := openai.New(llmOpts...)
	if err != nil {
		detail := "failed to initialize model for v1 openai interpreter"
		slog.ErrorContext(context.Background(), detail, "error", err)
		panic(detail)
	}

	d.llm = llm

	return d
}
