//go:build !unix

package tools

import "os/exec"

func inProcessGroup(*exec.Cmd)   {}
func killProcessGroup(*exec.Cmd) {}
