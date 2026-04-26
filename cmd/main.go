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
	{name: "run", description: "Extract schemas and upload when credentials are configured (default)"},
	{name: "extract", description: "Extract schemas from a Kubernetes cluster"},
	{name: "convert", description: "Convert CRD YAML files to JSON Schema"},
	{name: "upload", description: "Upload schemas to Cloudflare Pages"},
	{name: "watch", description: "Watch for CRD changes and upload when credentials are configured"},
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
	if arg == "--version" && len(osArgs) == 2 {
		return "version", nil
	}
	if arg == "-v" {
		return "version", nil
	}
	if isDefaultRunFlag(arg) {
		return "run", osArgs[1:]
	}
	return arg, osArgs[2:]
}

func isDefaultRunFlag(arg string) bool {
	if arg == "-o" || strings.HasPrefix(arg, "-o=") {
		return true
	}
	for _, name := range []string{"output-dir", "kind", "group", "version"} {
		if arg == "--"+name || strings.HasPrefix(arg, "--"+name+"=") {
			return true
		}
	}
	return false
}

type flagAlias struct {
	name  string
	alias string
}

var flagAliases = map[*flag.FlagSet][]flagAlias{}

func newCommandFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.Usage = func() {
		printFlagDefaults(fs)
	}
	return fs
}

func stringFlagWithAlias(fs *flag.FlagSet, target *string, name, alias, value, usage string) {
	fs.StringVar(target, name, value, usage)
	fs.StringVar(target, alias, value, usage)
	flagAliases[fs] = append(flagAliases[fs], flagAlias{name: name, alias: alias})
}

func printFlagDefaults(fs *flag.FlagSet) {
	output := fs.Output()
	fmt.Fprintf(output, "Usage of %s:\n", fs.Name())

	aliases := map[string]string{}
	aliasNames := map[string]bool{}
	for _, a := range flagAliases[fs] {
		aliases[a.name] = a.alias
		aliasNames[a.alias] = true
	}

	fs.VisitAll(func(f *flag.Flag) {
		if aliasNames[f.Name] {
			return
		}

		name, usage := flag.UnquoteUsage(f)
		displayName := "--" + f.Name
		if alias, ok := aliases[f.Name]; ok {
			displayName = "-" + alias + ", " + displayName
		}
		if name != "" {
			displayName += " " + name
		}

		fmt.Fprintf(output, "  %s\n", displayName)
		if usage != "" {
			fmt.Fprintf(output, "\t%s", usage)
			if f.DefValue != "" && f.DefValue != "false" {
				fmt.Fprintf(output, " (default %q)", f.DefValue)
			}
			fmt.Fprintln(output)
		}
	})
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
	fs := newCommandFlagSet(cmd)
	var outputDir string
	stringFlagWithAlias(fs, &outputDir, "output-dir", "o", fallback, "output directory")
	if err := fs.Parse(args); err != nil {
		return "", false, err
	}
	if extras := fs.Args(); len(extras) > 0 {
		return "", false, fmt.Errorf("unexpected arguments for %s: %s", cmd, strings.Join(extras, " "))
	}
	explicit := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "output-dir" || f.Name == "o" {
			explicit = true
		}
	})
	return outputDir, explicit, nil
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
