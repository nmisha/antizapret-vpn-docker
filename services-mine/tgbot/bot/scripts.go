package main

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
)

func envTrim(key string) string { return strings.TrimSpace(os.Getenv(key)) }

// success: возвращаем только stdout (stderr игнорируем)
// error: возвращаем stdout+stderr
func runScript(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	var outb, errb bytes.Buffer
	cmd.Stdout = &outb
	cmd.Stderr = &errb

	err := cmd.Run()

	stdout := strings.TrimSpace(outb.String())
	stderr := strings.TrimSpace(errb.String())

	if err == nil {
		if stdout == "" {
			return "(пустой вывод)", nil
		}
		return stdout, nil
	}

	combined := stdout
	if stderr != "" {
		if combined != "" {
			combined += "\n"
		}
		combined += stderr
	}
	if strings.TrimSpace(combined) == "" {
		combined = "(пустой вывод)"
	}
	return combined, err
}
