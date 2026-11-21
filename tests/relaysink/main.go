package main

import (
	"log"
	"os"
)

func main() {
	settings, err := SettingsFromEnvironment(os.Getenv)
	if err != nil {
		log.Fatal(err)
	}
	log.Fatal(RelaySink{Settings: settings}.Run())
}
