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
	"github.com/w-h-a/gomento/internal/client/interpreter"
)

type v1OpenaiInterpreter struct {
	options  interpreter.Options
	llm      llms.Model
	embedder embeddings.Embedder
}

func (d *v1OpenaiInterpreter) Distill(ctx context.Context, history []v1.Message) (*v1.Skill, error) {
	slog.InfoContext(ctx, "distilling session", "message_count", len(history))

	content := []llms.MessageContent{
		llms.TextParts(llms.ChatMessageTypeSystem, d.distillSystemPrompt()),
		llms.TextParts(llms.ChatMessageTypeHuman, d.distillUserPrompt(history)),
	}

	rsp, err := d.llm.GenerateContent(
		ctx,
		content,
		llms.WithJSONMode(),
	)
	if err != nil {
		return nil, fmt.Errorf("llm session distillation failed: %w", err)
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

func (d *v1OpenaiInterpreter) Extract(ctx context.Context, history []v1.Message, currentTasks []v1.Task) ([]interpreter.TaskAction, error) {
	slog.InfoContext(ctx, "extracting tasks", "msg_count", len(history), "current_task_count", len(currentTasks))

	content := []llms.MessageContent{
		llms.TextParts(llms.ChatMessageTypeSystem, d.extractSystemPrompt()),
		llms.TextParts(llms.ChatMessageTypeHuman, d.extractUserPrompt(history, currentTasks)),
	}

	rsp, err := d.llm.GenerateContent(
		ctx,
		content,
		llms.WithTools(d.getExtractTools()),
		llms.WithTemperature(0),
	)
	if err != nil {
		return nil, fmt.Errorf("llm task extraction failed: %w", err)
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

	return actions, nil
}

func (d *v1OpenaiInterpreter) distillSystemPrompt() string {
	return `You are an expert at distilling user intents into automation "Skills".
A Skill consists of two parts:
1. Trigger: A concise phrase describing WHEN this should happen.
2. SOP: A detailed description of WHAT should happen (Standard Operating Procedure).

Output MUST be a valid JSON object with keys "trigger" and "sop".
Example: {"trigger": "every morning at 9am", "sop": "check jira for high priority bugs"}`
}

func (d *v1OpenaiInterpreter) extractSystemPrompt() string {
	return `You are an expert at extracting "Tasks" and distinguishing tasks or actions from thought during a chat session. Your goal is to maintain a structured "Project Plan" (Task List) that accurately reflects the conversation history.

### Core Criteria
- **MECE (Mutually Exclusive, Collectively Exhaustive):** Your tasks should cover all work discussed without overlapping.
- **State Tracking:** You are the source of truth for what is "Pending", "Running", or "Success".
- **Evidence-Based:** You must link messages to tasks to prove why a status changed.

### Your Tools
1. **insert_task**:
   - Use this ONLY for *new* distinct units of work.
   - Do not create tasks for "Thinking" or "Talking" (use Thought for that).
   - Tasks must be actionable (e.g., "Fix Nginx Config", not "Think about Nginx").

2. **update_task**:
   - Use this to change status (e.g., pending -> success) or refine a description.
   - IMPORTANT: A task is "Success" ONLY when the user confirms it or the output is visible.

3. **append_messages_to_task**:
   - Use this to attach "Evidence".
   - IF a user says "I did X", attach that message to Task X.
   - IF you provide code for Y, attach that message to Task Y.

4. **append_messages_to_thought**:
   - Use this for "Meta-Context".
   - Attach messages about strategy, confusion, clarification, or future plans.
   - This keeps the actual Task List clean.

5. **finish**:
   - Call this ONLY when the Task List perfectly matches the conversation state.

### Execution Loop
1. **Read** the "Current Tasks" and "Messages".
2. **Think** step-by-step:
   - "Does the user's last message complete Task 2?" -> If yes, Update Task 2 Status + Append Message.
   - "Did the user ask for something new?" -> If yes, Insert Task.
   - "Is this just discussion?" -> Append to Thought.
3. **Act** by calling the appropriate tools.
`
}

func (d *v1OpenaiInterpreter) distillUserPrompt(history []v1.Message) string {
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

func (d *v1OpenaiInterpreter) extractUserPrompt(history []v1.Message, tasks []v1.Task) string {
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

	sb.WriteString("\n## Messages:\n")

	for i, msg := range history {
		var lines []string
		for _, p := range msg.Parts {
			lines = append(lines, d.packMessageLine(msg.Role, p))
		}
		sb.WriteString(fmt.Sprintf("<message id=%d> %s </message>\n", i, strings.Join(lines, "\n")))
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
