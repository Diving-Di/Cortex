package migrations

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const advisoryLockID int64 = 0x44494152594d4947

var cortexRoleReplacer = strings.NewReplacer(
	"diary_app", "cortex_app",
	"diary_migrator", "cortex_migrator",
)

var acceptedLegacyChecksums = map[int64]map[string]struct{}{
	10: {"e0fa4cb4fe22b92551fe9666520742e681870d79f76a1fffc776b1dddcb8936e": {}},
	11: {"01046b8740695e346c9399bf49df09eb19b9c03a71906473de9fab157cc667cc": {}},
	12: {"2b3b30f0f765b42e28af0b8e03359a6b235d76cf5c9dfcac6ac7caddedd7aa65": {}},
}

func executableSQL(sql string) string {
	return cortexRoleReplacer.Replace(sql)
}

func checksumAccepted(version int64, applied, current string) bool {
	if applied == current {
		return true
	}
	_, ok := acceptedLegacyChecksums[version][applied]
	return ok
}

//go:embed sql/*.sql
var migrationFiles embed.FS

type Migration struct {
	Version  int64
	Name     string
	UpSQL    string
	DownSQL  string
	Checksum string
}

type Applied struct {
	Version   int64
	Name      string
	Checksum  string
	AppliedAt time.Time
}

func Load() ([]Migration, error) {
	entries, err := fs.ReadDir(migrationFiles, "sql")
	if err != nil {
		return nil, err
	}
	values := map[int64]*Migration{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		parts := strings.Split(entry.Name(), ".")
		if len(parts) != 3 || (parts[1] != "up" && parts[1] != "down") || parts[2] != "sql" {
			return nil, fmt.Errorf("invalid migration filename %q", entry.Name())
		}
		identity := strings.SplitN(parts[0], "_", 2)
		if len(identity) != 2 {
			return nil, fmt.Errorf("invalid migration identity %q", entry.Name())
		}
		version, err := strconv.ParseInt(identity[0], 10, 64)
		if err != nil || version <= 0 {
			return nil, fmt.Errorf("invalid migration version %q", entry.Name())
		}
		migration := values[version]
		if migration == nil {
			migration = &Migration{Version: version, Name: identity[1]}
			values[version] = migration
		} else if migration.Name != identity[1] {
			return nil, fmt.Errorf("migration %d has conflicting names", version)
		}
		content, err := fs.ReadFile(migrationFiles, "sql/"+entry.Name())
		if err != nil {
			return nil, err
		}
		if parts[1] == "up" {
			migration.UpSQL = string(content)
		} else {
			migration.DownSQL = string(content)
		}
	}
	result := make([]Migration, 0, len(values))
	for _, migration := range values {
		if strings.TrimSpace(migration.UpSQL) == "" || strings.TrimSpace(migration.DownSQL) == "" {
			return nil, fmt.Errorf("migration %d requires up and down SQL", migration.Version)
		}
		digest := sha256.Sum256([]byte(migration.UpSQL))
		migration.Checksum = hex.EncodeToString(digest[:])
		result = append(result, *migration)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Version < result[j].Version })
	return result, nil
}

func WithLock(ctx context.Context, conn *pgx.Conn, action func() error) error {
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", advisoryLockID); err != nil {
		return fmt.Errorf("acquire migration advisory lock: %w", err)
	}
	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = conn.Exec(unlockCtx, "SELECT pg_advisory_unlock($1)", advisoryLockID)
	}()
	return action()
}

func EnsureTable(ctx context.Context, conn *pgx.Conn) error {
	_, err := conn.Exec(ctx, `
        CREATE TABLE IF NOT EXISTS public.schema_migrations (
            version bigint PRIMARY KEY,
            name text NOT NULL,
            checksum char(64) NOT NULL,
            applied_at timestamptz NOT NULL DEFAULT now()
        )`)
	return err
}

func AppliedVersions(ctx context.Context, conn *pgx.Conn) ([]Applied, error) {
	rows, err := conn.Query(ctx, `
        SELECT version, name, checksum, applied_at
        FROM public.schema_migrations ORDER BY version`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Applied
	for rows.Next() {
		var item Applied
		if err := rows.Scan(&item.Version, &item.Name, &item.Checksum, &item.AppliedAt); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func Up(ctx context.Context, conn *pgx.Conn, limit int) (int, error) {
	available, err := Load()
	if err != nil {
		return 0, err
	}
	applied, err := AppliedVersions(ctx, conn)
	if err != nil {
		return 0, err
	}
	known := make(map[int64]Applied, len(applied))
	for _, item := range applied {
		known[item.Version] = item
	}
	count := 0
	for _, migration := range available {
		if item, ok := known[migration.Version]; ok {
			if !checksumAccepted(migration.Version, item.Checksum, migration.Checksum) {
				return count, fmt.Errorf("migration %d checksum changed after application", migration.Version)
			}
			continue
		}
		if limit > 0 && count >= limit {
			break
		}
		tx, err := conn.Begin(ctx)
		if err != nil {
			return count, err
		}
		if _, err = tx.Exec(ctx, executableSQL(migration.UpSQL)); err == nil {
			_, err = tx.Exec(ctx, `INSERT INTO public.schema_migrations(version, name, checksum) VALUES ($1, $2, $3)`,
				migration.Version, migration.Name, migration.Checksum)
		}
		if err != nil {
			_ = tx.Rollback(ctx)
			return count, fmt.Errorf("apply migration %d_%s: %w", migration.Version, migration.Name, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func Down(ctx context.Context, conn *pgx.Conn, steps int) (int, error) {
	if steps <= 0 {
		return 0, fmt.Errorf("rollback steps must be positive")
	}
	available, err := Load()
	if err != nil {
		return 0, err
	}
	byVersion := make(map[int64]Migration, len(available))
	for _, migration := range available {
		byVersion[migration.Version] = migration
	}
	applied, err := AppliedVersions(ctx, conn)
	if err != nil {
		return 0, err
	}
	count := 0
	for index := len(applied) - 1; index >= 0 && count < steps; index-- {
		item := applied[index]
		migration, ok := byVersion[item.Version]
		if !ok {
			return count, fmt.Errorf("applied migration %d is missing from binary", item.Version)
		}
		tx, err := conn.Begin(ctx)
		if err != nil {
			return count, err
		}
		if _, err = tx.Exec(ctx, executableSQL(migration.DownSQL)); err == nil {
			_, err = tx.Exec(ctx, `DELETE FROM public.schema_migrations WHERE version = $1`, migration.Version)
		}
		if err != nil {
			_ = tx.Rollback(ctx)
			return count, fmt.Errorf("rollback migration %d_%s: %w", migration.Version, migration.Name, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}
