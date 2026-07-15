package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/lancekrogers/tcount/internal/cache"
	"github.com/lancekrogers/tcount/internal/errors"
)

func newCacheCommand() *cobra.Command {
	cacheCmd := &cobra.Command{
		Use:   "cache",
		Short: "Inspect or remove persistent directory cache state",
		Args:  cobra.NoArgs,
	}

	var statusJSON bool
	statusCmd := &cobra.Command{
		Use:   "status [path]",
		Short: "Show cache status for a directory",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCacheStatus(cmd.Context(), cacheCommandPath(args), statusJSON)
		},
	}
	statusCmd.Flags().BoolVar(&statusJSON, "json", false, "output status in JSON format")

	var clearAll bool
	clearCmd := &cobra.Command{
		Use:   "clear [path]",
		Short: "Remove cache state for a directory or all directories",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCacheClear(cmd.Context(), args, clearAll)
		},
	}
	clearCmd.Flags().BoolVar(&clearAll, "all", false, "clear all tcount cache state (explicit, noninteractive)")

	cacheCmd.AddCommand(statusCmd, clearCmd)
	return cacheCmd
}

func cacheCommandPath(args []string) string {
	if len(args) == 0 {
		return "."
	}
	return args[0]
}

func runCacheStatus(ctx context.Context, path string, jsonOutput bool) error {
	store, err := newCacheStore()
	if err != nil {
		return errors.Wrap(err, "creating cache store")
	}
	status, err := store.Status(ctx, path)
	if err != nil {
		return errors.Wrap(err, "reading cache status")
	}
	if jsonOutput {
		return outputCacheStatusJSON(status)
	}
	return outputCacheStatus(status)
}

func outputCacheStatus(status cache.Status) error {
	output := fmt.Sprintf("Cache Status\n  Root: %s\n  Present: %t\n  Schema: %d\n  Entries: %d\n  Bytes: %d\n  Generation: %d\n  Age: %s\n",
		status.Root,
		status.Present,
		status.SchemaVersion,
		status.Entries,
		status.Bytes,
		status.Generation,
		status.Age,
	)
	if !status.ModifiedAt.IsZero() {
		output += fmt.Sprintf("  Modified: %s\n", status.ModifiedAt.Format("2006-01-02T15:04:05.999999999Z07:00"))
	}
	if _, err := fmt.Fprint(os.Stdout, output); err != nil {
		return errors.Wrap(err, "writing cache status")
	}
	return nil
}

type cacheStatusJSON struct {
	Root          string `json:"root"`
	Present       bool   `json:"present"`
	Failure       string `json:"failure,omitempty"`
	SchemaVersion uint32 `json:"schema_version"`
	Entries       int    `json:"entries"`
	Bytes         int64  `json:"bytes"`
	Generation    uint64 `json:"generation"`
	Age           string `json:"age"`
	ModifiedAt    string `json:"modified_at,omitempty"`
}

func outputCacheStatusJSON(status cache.Status) error {
	modifiedAt := ""
	if !status.ModifiedAt.IsZero() {
		modifiedAt = status.ModifiedAt.Format("2006-01-02T15:04:05.999999999Z07:00")
	}
	output := cacheStatusJSON{
		Root:          status.Root,
		Present:       status.Present,
		Failure:       string(status.Failure),
		SchemaVersion: status.SchemaVersion,
		Entries:       status.Entries,
		Bytes:         status.Bytes,
		Generation:    status.Generation,
		Age:           status.Age.String(),
		ModifiedAt:    modifiedAt,
	}
	return json.NewEncoder(os.Stdout).Encode(output)
}

func runCacheClear(ctx context.Context, args []string, all bool) error {
	if all && len(args) > 0 {
		return errors.Validation("--all cannot be combined with a cache path")
	}
	store, err := newCacheStore()
	if err != nil {
		return errors.Wrap(err, "creating cache store")
	}
	if all {
		if err := store.ClearAll(ctx); err != nil {
			return errors.Wrap(err, "clearing all cache state")
		}
		if _, err := fmt.Fprintln(os.Stdout, "Cleared all tcount cache state"); err != nil {
			return errors.Wrap(err, "writing cache clear result")
		}
		return nil
	}

	path := cacheCommandPath(args)
	canonical, err := cache.CanonicalRoot(path)
	if err != nil {
		return errors.Wrap(err, "resolving cache root")
	}
	if err := store.Clear(ctx, canonical); err != nil {
		return errors.Wrap(err, "clearing cache state")
	}
	if _, err := fmt.Fprintf(os.Stdout, "Cleared tcount cache state for %s\n", canonical); err != nil {
		return errors.Wrap(err, "writing cache clear result")
	}
	return nil
}
