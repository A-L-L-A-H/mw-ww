package guard

import (
	"encoding/json"
	"strings"
)

// parseProcessJSON parses a JSON array of {ProcessName, Id} objects.
func parseProcessJSON(text string) ([]runningProc, error) {
	type rawProc struct {
		ProcessName string      `json:"ProcessName"`
		Id          interface{} `json:"Id"`
	}
	var raws []rawProc
	if err := json.Unmarshal([]byte(text), &raws); err != nil {
		return nil, err
	}
	out := make([]runningProc, 0, len(raws))
	for _, r := range raws {
		if r.ProcessName == "" || r.Id == nil {
			continue
		}
		id := ""
		switch v := r.Id.(type) {
		case float64:
			id = trimFloat(v)
		case string:
			id = v
		}
		if id == "" {
			continue
		}
		out = append(out, runningProc{Name: r.ProcessName, PID: id})
	}
	return out, nil
}

func trimFloat(f float64) string {
	s := strings.TrimRight(strings.TrimRight(
		strings.TrimSpace(jsonNumber(f)), "0"), ".")
	if s == "" {
		return "0"
	}
	return s
}

func jsonNumber(f float64) string {
	b, _ := json.Marshal(f)
	return string(b)
}
