package main

import (
	"fmt"
	"os"
	"os/exec"
)

func main() {
	fmt.Println("glassroot gvisor parent fixture")
	cmd := exec.Command("/glassroot-child")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		os.Exit(3)
	}
}
