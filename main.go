package main

import (
	"log"
	"os/exec"

	"github.com/bernard-sh/tfs/cmd"
)

func main() {
	_, err := exec.LookPath("terraform")
	if err != nil {
		log.Fatal(err)
	}
	cmd.Execute()
}