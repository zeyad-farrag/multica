// Package migrations embeds the team-app migration SQL files so the boot path can
// apply them without depending on the working directory layout. The migration
// runner (team-app/internal/migrate) reads from FS in lex order; tests in this
// package use FS directly via the helper functions below.
package migrations

import (
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

//go:embed *.sql
var FS embed.FS

// UpFiles returns all *.up.sql filenames in lexical order.
func UpFiles() ([]string, error) {
	return filesWithSuffix(".up.sql", false)
}

// DownFiles returns all *.down.sql filenames in REVERSE lexical order so the
// caller can roll back from newest to oldest.
func DownFiles() ([]string, error) {
	return filesWithSuffix(".down.sql", true)
}

// Read returns the contents of an embedded migration file.
func Read(name string) ([]byte, error) {
	return fs.ReadFile(FS, name)
}

func filesWithSuffix(suffix string, reverse bool) ([]string, error) {
	entries, err := fs.ReadDir(FS, ".")
	if err != nil {
		return nil, fmt.Errorf("read embedded migrations: %w", err)
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, suffix) {
			out = append(out, name)
		}
	}
	if reverse {
		sort.Sort(sort.Reverse(sort.StringSlice(out)))
	} else {
		sort.Strings(out)
	}
	return out, nil
}
