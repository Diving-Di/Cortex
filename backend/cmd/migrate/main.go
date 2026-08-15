package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"cortex/backend/internal/migrations"
	"github.com/jackc/pgx/v5"
)

func main() {
	steps := flag.Int("steps", 1, "number of migrations for up/down; 0 means all for up")
	timeout := flag.Duration("timeout", 5*time.Minute, "total migration timeout")
	flag.Parse()
	command := "status"
	if flag.NArg() > 0 {
		command = strings.ToLower(flag.Arg(0))
	}
	databaseURL := strings.TrimSpace(os.Getenv("MIGRATION_DATABASE_URL"))
	if databaseURL == "" {
		fatal("MIGRATION_DATABASE_URL is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		fatal("connect migration database: %v", err)
	}
	defer conn.Close(context.Background())
	err = migrations.WithLock(ctx, conn, func() error {
		if err := migrations.EnsureTable(ctx, conn); err != nil {
			return err
		}
		switch command {
		case "up":
			count, err := migrations.Up(ctx, conn, *steps)
			if err == nil {
				fmt.Printf("applied %d migration(s)\n", count)
			}
			return err
		case "down":
			count, err := migrations.Down(ctx, conn, *steps)
			if err == nil {
				fmt.Printf("rolled back %d migration(s)\n", count)
			}
			return err
		case "status":
			applied, err := migrations.AppliedVersions(ctx, conn)
			if err != nil {
				return err
			}
			for _, item := range applied {
				fmt.Printf("%06d %-32s %s\n", item.Version, item.Name, item.AppliedAt.Format(time.RFC3339))
			}
			fmt.Printf("%d migration(s) applied\n", len(applied))
			return nil
		default:
			return fmt.Errorf("unknown command %q; use up, down, or status", command)
		}
	})
	if err != nil {
		fatal("%v", err)
	}
}

func fatal(format string, values ...any) {
	fmt.Fprintf(os.Stderr, "migration failed: "+format+"\n", values...)
	os.Exit(1)
}
