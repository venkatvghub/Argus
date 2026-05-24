package main

import (
	"github.com/joho/godotenv"
	"github.com/venkatvghub/argus/cmd/argus/cmd"
)

func main() {
	_ = godotenv.Load() // load .env if present; silently ignore if absent
	cmd.Execute()
}
