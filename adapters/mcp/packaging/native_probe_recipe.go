package packaging

import "encoding/json"

// Private qualification precedes current-client publication. All canonical
// rows must execute the MCP probe directly, retaining its complete admitted
// argv. Separately added fault/scope controls keep their existing commands.
func instrumentNativeProbeRows(hooks map[string]any, platform string, admittedArgs []string) {
	for event, rows := range hooks {
		for _, row := range rows.([]any) {
			for _, raw := range row.(map[string]any)["hooks"].([]any) {
				hook := raw.(map[string]any)
				hook["command"] = "${CLAUDE_PLUGIN_ROOT}/scripts/" + binaryName(platform)
				hook["args"] = append([]string{"hook", "--event", event}, admittedArgs[1:]...)
			}
		}
	}
}

// This is the final JSON passed to buildNativeCandidate, after specialized
// startup rows have been reinserted. Reset those canonical rows as well; the
// separate fault/scope rows and every already selected argv remain untouched.
func marshalNativeProbeHooks(config map[string]any, platform string) ([]byte, error) {
	for _, raw := range config["hooks"].(map[string]any) {
		row := raw.([]any)[0].(map[string]any)
		hook := row["hooks"].([]any)[0].(map[string]any)
		hook["command"] = "${CLAUDE_PLUGIN_ROOT}/scripts/" + binaryName(platform)
	}
	body, err := json.MarshalIndent(config, "", "  ")
	return append(body, '\n'), err
}
