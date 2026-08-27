package app

import (
	"strings"
	"testing"
)

func TestLocalRuntimeTasksLoadOptionalDotEnv(t *testing.T) {
	taskfile := readTask2Artifact(t, "Taskfile.yaml")
	for _, taskName := range []string{"run", "portal-preview"} {
		section := taskfileTaskSection(taskfile, taskName)
		if !strings.Contains(section, `dotenv: [".env"]`) {
			t.Errorf("task %q does not load the optional .env runtime configuration", taskName)
		}
	}
}

func taskfileTaskSection(taskfile, taskName string) string {
	lines := strings.Split(taskfile, "\n")
	header := "  " + taskName + ":"
	start := -1
	for index, line := range lines {
		if line == header {
			start = index
			break
		}
	}
	if start < 0 {
		return ""
	}
	end := len(lines)
	for index := start + 1; index < len(lines); index++ {
		line := lines[index]
		if strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "    ") && strings.HasSuffix(line, ":") {
			end = index
			break
		}
	}
	return strings.Join(lines[start:end], "\n")
}
