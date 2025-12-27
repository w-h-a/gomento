package interpreter

const (
	TaskActionInsert        = "insert_task"
	TaskActionUpdate        = "update_task"
	TaskActionAppendTask    = "append_messages_to_task"
	TaskActionAppendThought = "append_messages_to_thought"
	TaskActionFinish        = "finish"
)

type TaskAction struct {
	Type    string         `json:"type"`
	Payload map[string]any `json:"payload"`
}
