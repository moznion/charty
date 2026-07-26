// Command charty renders a time series, given as JSON, to a line chart.
//
//	charty -o coverage.png series.json
//	jq '[.[] | {t: .timestamp, v: .value}]' data.json | charty -f html -o out.html
//
// Input is either a chart object or a bare array of points; see the charty
// package documentation for the schema.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/moznion/charty"
)

var version = "dev"

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(2)
		}
		fmt.Fprintln(os.Stderr, "charty: "+err.Error())
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout io.Writer) error {
	var (
		format      string
		output      string
		gap         string
		xAxis       string
		yMin, yMax  string
		opts        charty.Options
		showVersion bool
	)
	fs := flag.NewFlagSet("charty", flag.ContinueOnError)
	fs.Usage = func() { usage(fs) }

	strVar(fs, &format, "format", "", "png, svg, html, csv or json (default: from -o, else png)", "f")
	strVar(fs, &output, "output", "", "output file, or - for stdout", "o")
	strVar(fs, &opts.Title, "title", "", "chart title, overriding the one in the input")
	fs.Float64Var(&opts.Width, "width", 0, "chart width in inches (default 7)")
	fs.Float64Var(&opts.Height, "height", 0, "chart height in inches (default 3.2)")
	fs.IntVar(&opts.DPI, "dpi", 0, "raster resolution for png (default 144)")
	strVar(fs, &xAxis, "x", "time", "x-axis placement: \"time\" by timestamp, or \"index\" at even spacing")
	fs.BoolVar(&opts.NoLabel, "no-label", false, "omit the value label on each series' latest point")
	strVar(fs, &gap, "gap", "", "break the line between points further apart than this, e.g. 7d")
	strVar(fs, &yMin, "y-min", "", "pin the bottom of the value axis")
	strVar(fs, &yMax, "y-max", "", "pin the top of the value axis")
	fs.BoolVar(&showVersion, "version", false, "print the version and exit")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if showVersion {
		_, _ = fmt.Fprintf(stdout, "charty %s\n", version)
		return nil
	}
	if fs.NArg() > 1 {
		return fmt.Errorf("expected at most one input file, got %d", fs.NArg())
	}

	format, err := resolveFormat(format, output)
	if err != nil {
		return err
	}
	opts.XAxis = charty.XAxis(xAxis)
	if opts.Gap, err = charty.ParseDuration(gap); err != nil {
		return fmt.Errorf("-gap: %w", err)
	}
	if opts.YMin, err = parseBound(yMin); err != nil {
		return fmt.Errorf("-y-min: %w", err)
	}
	if opts.YMax, err = parseBound(yMax); err != nil {
		return fmt.Errorf("-y-max: %w", err)
	}
	opts.Warn = func(msg string) { fmt.Fprintln(os.Stderr, "charty: "+msg) }

	in, closeIn, err := openInput(fs.Arg(0), stdin)
	if err != nil {
		return err
	}
	defer closeIn()

	chart, err := charty.Decode(in)
	if err != nil {
		return err
	}

	out, closeOut, err := openOutput(output, format, stdout)
	if err != nil {
		return err
	}
	defer closeOut()

	opts.Generator = "charty"
	return charty.Render(out, format, chart, opts)
}

// resolveFormat prefers an explicit -format, then the output file's extension.
// A chart with nowhere else to go defaults to png, which is what "render this"
// usually means.
func resolveFormat(format, output string) (string, error) {
	if format == "" {
		switch strings.ToLower(strings.TrimPrefix(filepath.Ext(output), ".")) {
		case "svg":
			format = "svg"
		case "html", "htm":
			format = "html"
		case "csv":
			format = "csv"
		case "json":
			format = "json"
		default:
			format = "png"
		}
	}
	for _, f := range charty.Formats {
		if format == f {
			return format, nil
		}
	}
	return "", fmt.Errorf("unknown format %q (want one of %s)", format, strings.Join(charty.Formats, ", "))
}

// parseBound reads an axis bound, distinguishing "unset" from zero.
func parseBound(s string) (*float64, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return nil, fmt.Errorf("cannot read %q as a number", s)
	}
	return &v, nil
}

func openInput(path string, stdin io.Reader) (io.Reader, func(), error) {
	if path == "" || path == "-" {
		return stdin, func() {}, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	return f, func() { _ = f.Close() }, nil
}

func openOutput(path, format string, stdout io.Writer) (io.Writer, func(), error) {
	if path == "" || path == "-" {
		if format == "png" {
			// A PNG on a terminal is line noise; ask for a file.
			if f, ok := stdout.(*os.File); ok && isTerminal(f) {
				return nil, nil, errors.New("refusing to write png to the terminal; pass -o FILE")
			}
		}
		return stdout, func() {}, nil
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, nil, err
	}
	return f, func() { _ = f.Close() }, nil
}

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// strVar registers a string flag under a canonical name plus any aliases.
func strVar(fs *flag.FlagSet, p *string, name, value, usage string, aliases ...string) {
	fs.StringVar(p, name, value, usage)
	for _, a := range aliases {
		fs.StringVar(p, a, value, "alias for -"+name)
	}
}

func usage(fs *flag.FlagSet) {
	w := fs.Output()
	_, _ = fmt.Fprint(w, `charty renders a time series, given as JSON, to a line chart.

Usage:
  charty [flags] [FILE]

Input is read from FILE or stdin, as either a chart object:

  {"title": "coverage",
   "series": [{"name": "coverage", "unit": "%",
               "points": [{"t": "2026-07-22T05:49:04Z", "v": 93.75,
                           "label": "ac01f467",
                           "href": "https://github.com/o/r/commit/ac01f467",
                           "meta": {"ref": "refs/heads/main"}}]}]}

or a bare array of points, which becomes a single unnamed series:

  [{"t": "2026-07-22T05:49:04Z", "v": 93.75}]

Only t and v are required. label, href and meta are carried through to the
html output, where a point can be clicked through to its link. A series may
also carry "gap": "7d", which breaks the line wherever measurements are
further apart than that rather than drawing straight through the silence.

-x index spaces points evenly instead of by timestamp, which keeps a history
measured in bursts readable at the cost of showing elapsed time.

png and svg text is drawn with a font that has no CJK glyphs; non-Latin
characters are reported and then dropped. Use html for those.

Flags:
`)
	fs.PrintDefaults()
}
