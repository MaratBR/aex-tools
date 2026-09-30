// Command gen writes internal/about/about.json: aex's version (winres/winres.json), its license
// (LICENSE, NOTICE) and the licenses of everything built into aex and its plugins, found from the
// source: the Go packages linked on Windows, macOS and Linux (go list -deps), with the license
// files from each package's folder up to its module's, plus the vendored and embedded files listed
// below. It fails on a license it does not know, or one aex (Apache-2.0) cannot ship with.
//
//	go generate ./internal/about      (writes about.json)
//	go run ./internal/about/gen -check (fails when about.json is out of date)
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// vendored are files copied into the repo from elsewhere, with their license file. Every file in
// a vendor folder must be listed here.
var vendored = []struct {
	Name, URL, License string
	Files              []string
}{{
	Name:    "xterm.js, @xterm/addon-fit",
	URL:     "https://github.com/xtermjs/xterm.js",
	License: "internal/webui/frontend/vendor/LICENSE-xterm",
	Files: []string{"internal/webui/frontend/vendor/xterm.js", "internal/webui/frontend/vendor/xterm.css",
		"internal/webui/frontend/vendor/addon-fit.js"},
}}

// vendorDirs are the folders vendored files are in.
var vendorDirs = []string{"internal/webui/frontend/vendor"}

// embedded are binaries a linked package builds in, whose license is not in its module.
var embedded = []struct {
	Name, URL, Package, License string
}{{
	Name:    "WebView2Loader.dll (Microsoft Edge WebView2 SDK)",
	URL:     "https://www.nuget.org/packages/Microsoft.Web.WebView2",
	Package: "github.com/wailsapp/go-webview2/webviewloader",
	License: "internal/about/notices/WebView2Loader.txt",
}}

// noLicenseFile are modules without a license file, and the license they state elsewhere.
var noLicenseFile = map[string]struct{ License, Note string }{
	"github.com/mattn/go-localereader": {"MIT", "No license file in the module; its README states: License: MIT."},
}

// allowed are the licenses aex, under Apache-2.0, can include: permissive ones, and MPL-2.0
// (file-level copyleft: its files stay under it, their source is linked).
var allowed = []string{"Apache-2.0", "MIT", "ISC", "BSD-2-Clause", "BSD-3-Clause", "0BSD", "Zlib", "Unlicense", "MPL-2.0"}

// targets are the systems aex builds for: each links different packages.
var targets = []string{"windows", "darwin", "linux"}

type file struct {
	Path string `json:"path"`
	Text string `json:"text"`
}

type component struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"` // empty for the Go standard library: the exe fills it in
	URL     string `json:"url"`
	License string `json:"license"` // SPDX expression
	Note    string `json:"note,omitempty"`
	Files   []file `json:"files,omitempty"`
}

type output struct {
	Version    string      `json:"version"`
	License    string      `json:"license"`
	Text       string      `json:"text"`
	Notice     string      `json:"notice"`
	ThirdParty []component `json:"thirdParty"`
}

type pkg struct {
	ImportPath string
	Dir        string
	Standard   bool
	Module     *struct {
		Path, Version, Dir string
		Main               bool
	}
	Error *struct{ Err string }
}

func main() {
	check := flag.Bool("check", false, "fail when about.json is out of date instead of writing it")
	flag.Parse()
	if err := run(*check); err != nil {
		fmt.Fprintln(os.Stderr, "about/gen:", err)
		os.Exit(1)
	}
}

func run(check bool) error {
	root, err := repoRoot()
	if err != nil {
		return err
	}
	out, err := generate(root)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	path := filepath.Join(root, "internal", "about", "about.json")
	if check {
		old, _ := os.ReadFile(path)
		if !bytes.Equal(bytes.ReplaceAll(old, []byte("\r\n"), []byte("\n")), data) {
			return errors.New("internal/about/about.json is out of date: run go generate ./internal/about")
		}
		return nil
	}
	return os.WriteFile(path, data, 0o644)
}

func repoRoot() (string, error) {
	b, err := exec.Command("go", "env", "GOMOD").Output()
	if err != nil {
		return "", fmt.Errorf("go env GOMOD: %w", err)
	}
	mod := strings.TrimSpace(string(b))
	if mod == "" || mod == os.DevNull {
		return "", errors.New("not in the aex module")
	}
	return filepath.Dir(mod), nil
}

func generate(root string) (output, error) {
	var out output
	var err error
	if out.Version, err = version(root); err != nil {
		return out, err
	}
	text, err := readText(filepath.Join(root, "LICENSE"))
	if err != nil {
		return out, err
	}
	if out.License = detect(text); out.License != "Apache-2.0" {
		return out, fmt.Errorf("LICENSE is %q, not Apache-2.0", out.License)
	}
	out.Text = text
	if out.Notice, err = readText(filepath.Join(root, "NOTICE")); err != nil {
		return out, err
	}

	pkgs, err := linked(root)
	if err != nil {
		return out, err
	}
	modules := map[string]*component{}
	var goFiles []file
	for _, p := range pkgs {
		if p.Standard {
			if goFiles == nil {
				goroot, err := goEnv("GOROOT")
				if err != nil {
					return out, err
				}
				if goFiles, err = licenseFiles(goroot, goroot); err != nil {
					return out, err
				}
			}
			continue
		}
		if p.Module == nil || p.Module.Main {
			continue
		}
		m := p.Module
		c := modules[m.Path]
		if c == nil {
			c = &component{Name: m.Path, Version: m.Version, URL: "https://pkg.go.dev/" + m.Path + "@" + m.Version}
			modules[m.Path] = c
		}
		files, err := licenseFiles(p.Dir, m.Dir)
		if err != nil {
			return out, err
		}
		c.Files = mergeFiles(c.Files, files)
	}
	for path, c := range modules {
		if len(c.Files) > 0 {
			continue
		}
		known, ok := noLicenseFile[path]
		if !ok {
			return out, fmt.Errorf("%s has no license file: check its license and add it to noLicenseFile", path)
		}
		c.License, c.Note = known.License, known.Note
	}
	for _, c := range modules {
		out.ThirdParty = append(out.ThirdParty, *c)
	}
	out.ThirdParty = append(out.ThirdParty, component{Name: "Go standard library", URL: "https://go.dev/LICENSE", Files: goFiles})

	for _, e := range embedded {
		if !slices.ContainsFunc(pkgs, func(p pkg) bool { return p.ImportPath == e.Package }) {
			continue
		}
		text, err := readText(filepath.Join(root, e.License))
		if err != nil {
			return out, err
		}
		out.ThirdParty = append(out.ThirdParty, component{Name: e.Name, URL: e.URL,
			Note:  "Built in by " + e.Package + ".",
			Files: []file{{filepath.Base(e.License), text}}})
	}

	listed := map[string]bool{}
	for _, v := range vendored {
		text, err := readText(filepath.Join(root, v.License))
		if err != nil {
			return out, err
		}
		listed[v.License] = true
		for _, f := range v.Files {
			if _, err := os.Stat(filepath.Join(root, f)); err != nil {
				return out, err
			}
			listed[f] = true
		}
		out.ThirdParty = append(out.ThirdParty, component{Name: v.Name, URL: v.URL,
			Note:  "Vendored: " + strings.Join(v.Files, ", ") + ".",
			Files: []file{{filepath.Base(v.License), text}}})
	}
	for _, dir := range vendorDirs {
		entries, err := os.ReadDir(filepath.Join(root, dir))
		if err != nil {
			return out, err
		}
		for _, e := range entries {
			if f := dir + "/" + e.Name(); !listed[f] {
				return out, fmt.Errorf("%s is not listed in vendored (internal/about/gen): add it with its license", f)
			}
		}
	}

	for i := range out.ThirdParty {
		c := &out.ThirdParty[i]
		if c.License == "" {
			if c.License, err = licenseOf(c.Files); err != nil {
				return out, fmt.Errorf("%s: %w", c.Name, err)
			}
		}
		if !compatible(c.License) {
			return out, fmt.Errorf("%s is under %s, which aex (Apache-2.0) cannot include", c.Name, c.License)
		}
	}
	slices.SortFunc(out.ThirdParty, func(a, b component) int { return strings.Compare(a.Name, b.Name) })
	return out, nil
}

// version is the ProductVersion in winres/winres.json.
func version(root string) (string, error) {
	var res struct {
		Version map[string]map[string]struct {
			Info map[string]struct{ ProductVersion string }
		} `json:"RT_VERSION"`
	}
	b, err := os.ReadFile(filepath.Join(root, "winres", "winres.json"))
	if err != nil {
		return "", err
	}
	if err := json.Unmarshal(b, &res); err != nil {
		return "", fmt.Errorf("winres.json: %w", err)
	}
	for _, langs := range res.Version {
		for _, v := range langs {
			for _, info := range v.Info {
				if info.ProductVersion != "" {
					return info.ProductVersion, nil
				}
			}
		}
	}
	return "", errors.New("winres.json has no ProductVersion")
}

// linked lists the packages aex and its plugins link, on every target, each once.
func linked(root string) ([]pkg, error) {
	seen := map[string]bool{}
	var all []pkg
	for _, goos := range targets {
		cmd := exec.Command("go", "list", "-deps", "-json", "-tags", "desktop,production", ".", "./plugins/...")
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "GOOS="+goos, "GOARCH=amd64", "CGO_ENABLED=1")
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		b, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("go list (%s): %v: %s", goos, err, stderr.String())
		}
		dec := json.NewDecoder(bytes.NewReader(b))
		for {
			var p pkg
			if err := dec.Decode(&p); err == io.EOF {
				break
			} else if err != nil {
				return nil, fmt.Errorf("go list (%s): %w", goos, err)
			}
			if p.Error != nil {
				return nil, fmt.Errorf("go list (%s): %s: %s", goos, p.ImportPath, p.Error.Err)
			}
			if !seen[p.ImportPath] {
				seen[p.ImportPath] = true
				all = append(all, p)
			}
		}
	}
	return all, nil
}

func goEnv(name string) (string, error) {
	b, err := exec.Command("go", "env", name).Output()
	if err != nil {
		return "", fmt.Errorf("go env %s: %w", name, err)
	}
	return strings.TrimSpace(string(b)), nil
}

var licenseName = regexp.MustCompile(`(?i)^(licen[cs]e|copying|notice|patents|unlicense)([.-].*)?$`)

// licenseFiles are the license files in dir and each folder above it up to top, named by their
// path from top (forward slashes).
func licenseFiles(dir, top string) ([]file, error) {
	var files []file
	for {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if e.IsDir() || !licenseName.MatchString(e.Name()) {
				continue
			}
			text, err := readText(filepath.Join(dir, e.Name()))
			if err != nil {
				return nil, err
			}
			rel, err := filepath.Rel(top, filepath.Join(dir, e.Name()))
			if err != nil {
				return nil, err
			}
			files = append(files, file{filepath.ToSlash(rel), text})
		}
		if filepath.Clean(dir) == filepath.Clean(top) {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil, fmt.Errorf("%s is not inside %s", dir, top)
		}
		dir = parent
	}
	return files, nil
}

func mergeFiles(have, more []file) []file {
	for _, f := range more {
		if !slices.ContainsFunc(have, func(h file) bool { return h.Path == f.Path }) {
			have = append(have, f)
		}
	}
	slices.SortFunc(have, func(a, b file) int {
		// The module's own (top folder) files first, then by path.
		if da, db := strings.Count(a.Path, "/"), strings.Count(b.Path, "/"); da != db {
			return da - db
		}
		return strings.Compare(a.Path, b.Path)
	})
	return have
}

func readText(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.ReplaceAll(string(b), "\r\n", "\n"), nil
}

// licenseOf is the licenses of files together (A AND B when they differ). Files that name no
// license (PATENTS, NOTICE) are left out, but at least one must name one.
func licenseOf(files []file) (string, error) {
	var ids []string
	for _, f := range files {
		id := detect(f.Text)
		if id == "" {
			if strings.HasPrefix(strings.ToUpper(filepath.Base(f.Path)), "PATENTS") ||
				strings.HasPrefix(strings.ToUpper(filepath.Base(f.Path)), "NOTICE") {
				continue
			}
			return "", fmt.Errorf("%s: license not recognised: check it and teach detect (internal/about/gen)", f.Path)
		}
		if !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return "", errors.New("no license found in its license files")
	}
	if len(ids) == 1 {
		return ids[0], nil
	}
	for i, id := range ids {
		if strings.Contains(id, " OR ") {
			ids[i] = "(" + id + ")"
		}
	}
	return strings.Join(ids, " AND "), nil
}

var spdx = regexp.MustCompile(`SPDX-License-Identifier:\s*([^\n]+)`)

// detect names the license of a license file's text (an SPDX expression), or "" when it does not
// know it.
func detect(text string) string {
	if m := spdx.FindStringSubmatch(text); m != nil {
		return strings.TrimSpace(m[1])
	}
	t := strings.Join(strings.Fields(text), " ")
	has := func(s string) bool { return strings.Contains(t, s) }
	switch {
	case has("Apache License") && has("Version 2.0"):
		return "Apache-2.0"
	case has("Mozilla Public License Version 2.0") || has("Mozilla Public License, version 2.0"):
		return "MPL-2.0"
	case has("GNU") && has("General Public License"):
		return "GPL" // not allowed, but named so the error says what it is
	case has("This is free and unencumbered software released into the public domain"):
		return "Unlicense"
	case has("Permission is hereby granted, free of charge"):
		return "MIT"
	case has("Permission to use, copy, modify, and/or distribute this software for any purpose") ||
		has("Permission to use, copy, modify, and distribute this software for any purpose"):
		return "ISC"
	case has("Redistribution and use in source and binary forms"):
		if has("Neither the name") || has("names of its contributors may not be used") || has("name of the author may not be used") {
			return "BSD-3-Clause"
		}
		return "BSD-2-Clause"
	}
	return ""
}

// compatible says whether aex can include something under the SPDX expression expr: every AND
// part must be allowed, an OR part needs only one of its choices.
func compatible(expr string) bool {
	for _, part := range strings.Split(expr, " AND ") {
		part = strings.Trim(strings.TrimSpace(part), "()")
		ok := false
		for _, choice := range strings.Split(part, " OR ") {
			if slices.Contains(allowed, strings.TrimSpace(choice)) {
				ok = true
			}
		}
		if !ok {
			return false
		}
	}
	return true
}
