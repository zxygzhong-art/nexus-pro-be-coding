// Command migrate is a thin golang-migrate CLI wrapper.
//
//	go run ./cmd/migrate up         # apply all up migrations
//	go run ./cmd/migrate down 1     # roll back N steps (default all)
//	go run ./cmd/migrate force 13   # set version, clear dirty flag
//	go run ./cmd/migrate version    # print current version
package main

import (
	"errors"
	"log"
	"os"
	"strconv"

	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/config"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("usage: migrate <up|down|force|version> [n]")
	}
	cfg := config.Load()

	sourceURL := "file://migrations"
	if v := os.Getenv("MIGRATIONS_PATH"); v != "" {
		sourceURL = "file://" + v
	}

	m, err := migrate.New(sourceURL, cfg.MigrateDsn)
	if err != nil {
		log.Fatalf("migrate init: %v", err)
	}
	defer m.Close()

	cmd := os.Args[1]
	switch cmd {
	case "up":
		err = m.Up()
	case "down":
		if len(os.Args) >= 3 {
			n, perr := strconv.Atoi(os.Args[2])
			if perr != nil {
				log.Fatalf("invalid step count: %v", perr)
			}
			err = m.Steps(-n)
		} else {
			err = m.Down()
		}
	case "force":
		if len(os.Args) < 3 {
			log.Fatal("force requires a version")
		}
		v, perr := strconv.Atoi(os.Args[2])
		if perr != nil {
			log.Fatalf("invalid version: %v", perr)
		}
		err = m.Force(v)
	case "version":
		v, dirty, verr := m.Version()
		if verr != nil {
			log.Fatalf("version: %v", verr)
		}
		log.Printf("version=%d dirty=%v", v, dirty)
		return
	default:
		log.Fatalf("unknown command: %s", cmd)
	}

	if err != nil && !errors.Is(err, migrate.ErrNoChange) {
		log.Fatalf("migrate %s: %v", cmd, err)
	}
	log.Printf("migrate %s: ok", cmd)
}
