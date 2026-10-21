package main

import (
	"embed"
	"log"

	"tunnelkit/internal/app"
)

//go:embed all:frontend/dist all:assets/icons build/config.yml
var assets embed.FS

func main() {
	if err := app.Run(assets); err != nil {
		log.Fatal(err)
	}
}
