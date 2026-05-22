package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/figarocorso/prowl/internal/data"
	"github.com/figarocorso/prowl/internal/ui"
	"github.com/spf13/cobra"
)

var addCmd = &cobra.Command{
	Use:   "add [URL...]",
	Short: "Track one or more PR URLs",
	Long: `Add tracks new PR URLs in the active list.

With no arguments, add reads URLs from stdin (one per line, blank to finish
when stdin is a TTY). Duplicates are skipped silently.`,
	RunE: runAdd,
}

func init() {
	rootCmd.AddCommand(addCmd)
}

func runAdd(cmd *cobra.Command, args []string) error {
	_, store, err := loadConfigAndStore()
	if err != nil {
		return err
	}

	if len(args) == 0 {
		args = readURLsFromStdin(cmd.InOrStdin(), cmd.OutOrStderr())
	}

	plainErr := ui.IsPlain(cmd.OutOrStderr())
	plainOut := ui.IsPlain(cmd.OutOrStdout())
	added := 0
	for _, raw := range args {
		canonical, err := data.CanonicalURL(strings.TrimSpace(raw))
		if err != nil {
			fmt.Fprintf(cmd.OutOrStderr(), "%s %s\n", ui.Err(plainErr), err)
			continue
		}
		ok, err := store.Add(canonical)
		if err != nil {
			return err
		}
		if !ok {
			fmt.Fprintf(cmd.OutOrStderr(), "%s already tracked: %s\n", ui.Warn(plainErr), canonical)
			continue
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%s added: %s\n", ui.OK(plainOut), canonical)
		added++
	}
	if added == 0 {
		fmt.Fprintln(cmd.OutOrStderr(), "no PRs added.")
	}
	return nil
}

func readURLsFromStdin(in interface{ Read([]byte) (int, error) }, errOut interface {
	Write([]byte) (int, error)
}) []string {
	scanner := bufio.NewScanner(in)
	var urls []string
	if isatty(os.Stdin) {
		fmt.Fprintln(errOut, "Enter PR URLs (one per line, empty line to finish):")
	}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			if isatty(os.Stdin) {
				break
			}
			continue
		}
		urls = append(urls, line)
	}
	return urls
}

func isatty(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}
