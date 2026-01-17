package interpreter

const (
	SkillActionInsert = "insert_skill"
	SkillActionUpdate = "update_skill"
	SkillActionFinish = "finish"
)

type SkillAction struct {
	Type    string         `json:"type"`
	Payload map[string]any `json:"payload"`
}
