package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/figarocorso/prowl/internal/config"
	"github.com/figarocorso/prowl/internal/ui"
	"github.com/spf13/cobra"
)

var checkCmd = &cobra.Command{
	Use:     "check",
	Aliases: []string{"check-dependencies", "doctor"},
	Short:   "Verify environment + auth + writable data dir",
	RunE:    runCheck,
}

func init() {
	rootCmd.AddCommand(checkCmd)
}

func runCheck(cmd *cobra.Command, _ []string) error {
	out := cmd.OutOrStdout()
	plain := ui.IsPlain(out)
	if plain {
		fmt.Fprintln(out, "prowl · environment check")
	} else {
		fmt.Fprintf(out, "%s %s\n", "🦉", ui.Title(plain, "prowl · environment check"))
	}
	fmt.Fprintln(out)

	missing := 0
	if path, err := exec.LookPath("gh"); err == nil {
		printOK(out, plain, "gh", path)
	} else {
		printMissing(out, plain, "gh", "install GitHub CLI (https://cli.github.com)")
		missing++
	}

	if err := ghAuthStatus(); err == nil {
		printOK(out, plain, "gh auth", "authenticated")
	} else {
		printOptional(out, plain, "gh auth", "run 'gh auth login' (private repos need auth)")
	}

	if path := browserOpener(); path != "" {
		printOK(out, plain, "browser", path)
	} else {
		printOptional(out, plain, "browser", "no 'open'/'xdg-open'; PR links won't auto-open")
	}

	if path := clipboardTool(); path != "" {
		printOK(out, plain, "clipboard", path)
	} else {
		printOptional(out, plain, "clipboard", "no pbcopy/wl-copy/xclip/xsel; copy disabled")
	}

	cfg, err := config.Load(dataDir)
	if err != nil {
		printMissing(out, plain, "data dir", err.Error())
		missing++
	} else if err := checkWritable(cfg.Paths.DataDir); err == nil {
		printOK(out, plain, "data dir", cfg.Paths.DataDir)
	} else {
		printMissing(out, plain, "data dir", err.Error())
		missing++
	}

	fmt.Fprintln(out)
	if missing > 0 {
		return fmt.Errorf("%d required dependency/dependencies missing", missing)
	}
	fmt.Fprintf(out, "%s all required dependencies present\n", ui.OK(plain))
	return nil
}

func ghAuthStatus() error {
	gh, err := exec.LookPath("gh")
	if err != nil {
		return err
	}
	cmd := exec.Command(gh, "auth", "status")
	cmd.Stderr = nil
	cmd.Stdout = nil
	return cmd.Run()
}

func browserOpener() string {
	if runtime.GOOS == "darwin" {
		if p, err := exec.LookPath("open"); err == nil {
			return p
		}
	}
	if p, err := exec.LookPath("xdg-open"); err == nil {
		return p
	}
	if v := os.Getenv("BROWSER"); v != "" {
		return v
	}
	return ""
}

func clipboardTool() string {
	for _, c := range []string{"pbcopy", "wl-copy", "xclip", "xsel"} {
		if p, err := exec.LookPath(c); err == nil {
			return p
		}
	}
	return ""
}

func checkWritable(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".prowl-write-")
	if err != nil {
		return fmt.Errorf("not writable: %w", err)
	}
	_ = tmp.Close()
	return os.Remove(tmp.Name())
}

func printOK(out interface{ Write([]byte) (int, error) }, plain bool, name, detail string) {
	if plain {
		fmt.Fprintf(out, "  [OK]       %-14s %s\n", name, detail)
		return
	}
	fmt.Fprintf(out, "  %s  %-14s %s\n", ui.OK(false), name, ui.Dim(false, detail))
}
func printMissing(out interface{ Write([]byte) (int, error) }, plain bool, name, detail string) {
	if plain {
		fmt.Fprintf(out, "  [MISSING]  %-14s %s\n", name, detail)
		return
	}
	fmt.Fprintf(out, "  %s  %-14s %s\n", ui.Err(false), name, detail)
}
func printOptional(out interface{ Write([]byte) (int, error) }, plain bool, name, detail string) {
	if plain {
		fmt.Fprintf(out, "  [OPTIONAL] %-14s %s\n", name, detail)
		return
	}
	fmt.Fprintf(out, "  %s  %-14s %s\n", ui.Warn(false), name, ui.Dim(false, detail))
}
