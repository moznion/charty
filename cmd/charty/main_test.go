package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

const points = `[{"t":"2026-07-01T00:00:00Z","v":91.5},{"t":"2026-07-02T00:00:00Z","v":93.75}]`

// Rendering an image is what "charty data.json" means, so png is the fallback
// when nothing names a format.
func TestResolveFormatDefaultsToPNG(t *testing.T) {
	for _, out := range []string{"", "-", "chart", "chart.unknown"} {
		got, err := resolveFormat("", out)
		if err != nil || got != "png" {
			t.Errorf("resolveFormat(\"\", %q) = %q, %v; want png", out, got, err)
		}
	}
}

func TestResolveFormatFollowsTheOutputExtension(t *testing.T) {
	for out, want := range map[string]string{
		"a.png": "png", "a.svg": "svg", "a.HTML": "html", "a.htm": "html",
		"a.csv": "csv", "a.json": "json", "dir/a.svg": "svg",
	} {
		got, err := resolveFormat("", out)
		if err != nil {
			t.Errorf("resolveFormat(%q): %v", out, err)
			continue
		}
		if got != want {
			t.Errorf("resolveFormat(%q) = %q, want %q", out, got, want)
		}
	}
}

func TestResolveFormatPrefersTheExplicitFlag(t *testing.T) {
	if got, err := resolveFormat("csv", "a.png"); err != nil || got != "csv" {
		t.Errorf("got %q, %v; want csv", got, err)
	}
}

func TestResolveFormatRejectsUnknownFormats(t *testing.T) {
	if _, err := resolveFormat("pdf", ""); err == nil {
		t.Error("expected an error")
	}
}

func TestRunReadsStdinAndWritesStdout(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"-f", "csv"}, strings.NewReader(points), &out); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out.String(), "93.75") {
		t.Errorf("output = %q", out.String())
	}
}

func TestRunRendersSVGToStdout(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"-f", "svg", "--title", "t"}, strings.NewReader(points), &out); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out.String(), "<svg") {
		t.Error("output is not an SVG")
	}
}

func TestRunReadsAFileArgument(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/points.json"
	if err := writeFile(path, points); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run([]string{"-f", "json", path}, strings.NewReader(""), &out); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out.String(), "93.75") {
		t.Errorf("output = %q", out.String())
	}
}

func TestRunRejectsSeveralInputFiles(t *testing.T) {
	if err := run([]string{"a.json", "b.json"}, strings.NewReader(""), &bytes.Buffer{}); err == nil {
		t.Error("expected an error")
	}
}

func TestRunReportsBadInput(t *testing.T) {
	if err := run([]string{"-f", "csv"}, strings.NewReader("not json"), &bytes.Buffer{}); err == nil {
		t.Error("expected an error")
	}
}

func TestRunPrintsVersion(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"-version"}, strings.NewReader(""), &out); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.HasPrefix(out.String(), "charty ") {
		t.Errorf("output = %q", out.String())
	}
}

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}
