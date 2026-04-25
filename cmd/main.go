package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/sholdee/crd-schema-publisher/extractor"
	"github.com/sholdee/crd-schema-publisher/index"
	"github.com/sholdee/crd-schema-publisher/renderer"
)

// version is set at build time via -ldflags "-X main.version=..."
var version = "dev"

var (
	buildClientFunc        = extractor.BuildClient
	buildSiteFunc          = extractor.BuildSite
	validateOutputDirFunc  = extractor.ValidateOutputDir
	renderPreviewFunc      = renderer.RenderAll
	generatePreviewFunc    = index.Generate
	scaffoldPreviewFunc    func(string) error
	preparePreviewSiteFunc func(string, string, bool) (string, func(), error)
	publishOutputFunc      func(string) error
	commands               = map[string]func([]string) error{}
)

type commandSpec struct {
	name        string
	description string
}

var commandSpecs = []commandSpec{
	{name: "run", description: "Extract schemas and upload to Cloudflare Pages (default)"},
	{name: "extract", description: "Extract schemas from a Kubernetes cluster"},
	{name: "convert", description: "Convert CRD YAML files to JSON Schema"},
	{name: "upload", description: "Upload schemas to Cloudflare Pages"},
	{name: "watch", description: "Watch for CRD changes and publish automatically"},
	{name: "preview", description: "Serve a local preview of the documentation site"},
}

func registerCommand(name string, run func([]string) error) {
	commands[name] = run
}

func initLogger(cmd string) {
	var handler slog.Handler
	switch cmd {
	case "preview":
		handler = slog.NewTextHandler(os.Stderr, nil)
	default:
		handler = slog.NewJSONHandler(os.Stderr, nil)
	}
	slog.SetDefault(slog.New(handler))
}

func printUsage(w io.Writer) {
	fmt.Fprint(w, `crd-schema-publisher — CRD JSON Schema extraction and documentation

Usage:
  crd-schema-publisher <command> [flags]

Commands:
`)
	for _, cmd := range commandSpecs {
		fmt.Fprintf(w, "  %-8s  %s\n", cmd.name, cmd.description)
	}
	fmt.Fprint(w, `
Use "crd-schema-publisher <command> --help" for more information.
`)
}

func parseSubcommand(osArgs []string) (string, []string) {
	if len(osArgs) < 2 {
		return "run", nil
	}
	arg := osArgs[1]
	if arg == "--help" || arg == "-h" {
		return "help", nil
	}
	if arg == "--version" || arg == "-v" {
		return "version", nil
	}
	if arg == "--output-dir" || strings.HasPrefix(arg, "--output-dir=") {
		return "run", osArgs[1:]
	}
	return arg, osArgs[2:]
}

func handleCmdError(cmd string, err error) {
	if err == nil || errors.Is(err, flag.ErrHelp) {
		return
	}
	slog.Error("command failed", "command", cmd, "error", err)
	os.Exit(1)
}

func main() {
	cmd, args := parseSubcommand(os.Args)
	initLogger(cmd)

	switch cmd {
	case "help":
		printUsage(os.Stdout)
		return
	case "version":
		fmt.Println(version)
		return
	default:
		run, ok := commands[cmd]
		if ok {
			handleCmdError(cmd, run(args))
			return
		}
		slog.Error("unknown command", "command", cmd)
		printUsage(os.Stderr)
		os.Exit(1)
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func requireEnv(key string) (string, error) {
	v := os.Getenv(key)
	if v == "" {
		return "", fmt.Errorf("required environment variable %s is not set", key)
	}
	return v, nil
}

func parseOutputDirArg(cmd string, args []string, fallback string) (string, bool, error) {
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	outputDir := fs.String("output-dir", fallback, "output directory")
	if err := fs.Parse(args); err != nil {
		return "", false, err
	}
	if extras := fs.Args(); len(extras) > 0 {
		return "", false, fmt.Errorf("unexpected arguments for %s: %s", cmd, strings.Join(extras, " "))
	}
	explicit := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "output-dir" {
			explicit = true
		}
	})
	return *outputDir, explicit, nil
}

func requireExistingOutputDir(outputDir, guidance string) error {
	if strings.TrimSpace(outputDir) == "" {
		return fmt.Errorf("OUTPUT_DIR must not be empty; it must already exist as a directory before running this command. %s", guidance)
	}
	if err := extractor.ValidateOutputDir(outputDir); err != nil {
		return fmt.Errorf("%w. Runtime commands require OUTPUT_DIR to already exist as a directory. %s", err, guidance)
	}
	info, err := os.Stat(outputDir)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("OUTPUT_DIR %q does not exist; it must already exist as a directory before running this command. %s", outputDir, guidance)
		}
		return fmt.Errorf("checking OUTPUT_DIR %q: %w", outputDir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("OUTPUT_DIR %q is not a directory; it must already exist as a directory before running this command. %s", outputDir, guidance)
	}
	return nil
}

func requireConfiguredOutputDir(outputDir, guidance string) error {
	if strings.TrimSpace(outputDir) == "" {
		return fmt.Errorf("OUTPUT_DIR is required for this command; set OUTPUT_DIR or pass --output-dir. %s", guidance)
	}
	if err := extractor.ValidateOutputDir(outputDir); err != nil {
		return fmt.Errorf("%w. %s", err, guidance)
	}
	return nil
}

func normalizeBasePath(s string) string {
	s = strings.TrimRight(s, "/")
	if s == "" {
		return ""
	}
	if !strings.HasPrefix(s, "/") {
		s = "/" + s
	}
	return s
}
