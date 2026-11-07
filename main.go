package main

import (
	"context"
	"embed"
	"log"
	"os"

	"tunnelkit/internal/app"
	"tunnelkit/internal/update"
)

//go:embed all:frontend/dist all:assets/icons build/config.yml
var assets embed.FS

func main() {
	if err := update.WaitForRestartParent(context.Background(), os.Args[1:]); err != nil {
		log.Fatal(err)
	}
	if err := app.Run(assets); err != nil {
		log.Fatal(err)
	}
}
