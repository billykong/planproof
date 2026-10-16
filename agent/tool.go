package agent

import "encoding/json"

// ToolName is the one tool the agent has.
const ToolName = "submit_plan"

// ToolDescription is the same for every model.
const ToolDescription = "Submit a plan to the kernel. Returns the kernel's verdict: Proved, or the failures (rule, step, evidence) or the Unknown trigger to fix."

// ToolSchema is the JSON Schema of the plan body. It guides the model;
// the kernel's strict parser is still what decides (rule input).
var ToolSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "goal": {"type": "array", "description": "Exactly the changes the plan makes.", "items": {"type": "object", "properties": {
      "node": {"type": "string"}, "deployment": {"type": "string", "description": "namespace/name"},
      "field": {"type": "string", "enum": ["unschedulable", "drained", "replicas", "image", "restarted"]},
      "container": {"type": "string"}, "from": {}, "to": {}}, "required": ["field", "from", "to"]}},
    "steps": {"type": "array", "items": {"type": "object", "properties": {
      "op": {"type": "string", "enum": ["cordon", "uncordon", "drain", "scale", "set-image", "rollout-restart", "wait"]},
      "node": {"type": "string"}, "deployment": {"type": "string"}, "replicas": {"type": "integer"},
      "container": {"type": "string"}, "image": {"type": "string"},
      "deployments": {"type": "array", "items": {"type": "string"}}, "timeoutSeconds": {"type": "integer"},
      "pre": {"type": "object"}, "post": {"type": "object"}}, "required": ["op"]}},
    "rollback": {"type": "array", "items": {"type": "object"}},
    "rationale": {"type": "string"}
  },
  "required": ["goal", "steps"]
}`)

// toolSchemaMap is the schema as a map, for clients that want one.
func toolSchemaMap() map[string]any {
	var m map[string]any
	json.Unmarshal(ToolSchema, &m)
	return m
}
