package main

import (
	"archive/zip"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
)

// wellKnownModules maps non-GitHub module prefixes to their actual GitHub paths.
var wellKnownModules = map[string]string{
	"golang.org/x/":              "github.com/golang/",
	"google.golang.org/grpc":     "github.com/grpc/grpc-go",
	"google.golang.org/protobuf": "github.com/protocolbuffers/protobuf-go",
	"google.golang.org/api":      "github.com/googleapis/google-api-go-client",
	"cloud.google.com/go":        "github.com/googleapis/google-cloud-go",
	"go.uber.org/":               "github.com/uber-go/",
	"go.opentelemetry.io/otel":   "github.com/open-telemetry/opentelemetry-go",
}

type githubLicense struct {
	Name   string `json:"name"`   // e.g. "MIT License"
	SPDXId string `json:"spdx_id"` // e.g. "MIT", "Apache-2.0"
}

type githubRepo struct {
	Description string         `json:"description"`
	FullName    string         `json:"full_name"`
	HTMLURL     string         `json:"html_url"`
	Homepage    string         `json:"homepage"`
	Topics      []string       `json:"topics"`
	License     githubLicense  `json:"license"`
}

type githubReadme struct {
	Content  string `json:"content"`
	Encoding string `json:"encoding"`
}

// githubBlobRe matches https://github.com/{owner}/{repo}/blob/{ref}/{path}
var githubBlobRe = regexp.MustCompile(`^https://github\.com/([^/]+/[^/]+)/blob/(.+)$`)

// fetchModContent reads a go.mod from a file path or HTTPS URL.
// GitHub "blob" URLs are rewritten to raw.githubusercontent.com automatically.
func fetchModContent(client *http.Client, src string) (data []byte, label string, err error) {
	if !strings.HasPrefix(src, "https://") {
		data, err = os.ReadFile(src)
		return data, src, err
	}

	rawURL := src
	if m := githubBlobRe.FindStringSubmatch(src); m != nil {
		rawURL = "https://raw.githubusercontent.com/" + m[1] + "/" + m[2]
	}

	resp, err := client.Get(rawURL)
	if err != nil {
		return nil, rawURL, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, rawURL, fmt.Errorf("HTTP %d fetching %s", resp.StatusCode, rawURL)
	}
	data, err = io.ReadAll(resp.Body)
	return data, rawURL, err
}

// metaContentRe extracts the content attribute of a named meta tag,
// handling either attribute order.
func metaContentRe(name string) *regexp.Regexp {
	escaped := regexp.QuoteMeta(name)
	return regexp.MustCompile(`<meta[^>]+name=["']` + escaped + `["'][^>]+content=["']([^"']+)["']|<meta[^>]+content=["']([^"']+)["'][^>]+name=["']` + escaped + `["']`)
}

var goImportRe = metaContentRe("go-import")
var goSourceRe = metaContentRe("go-source")

// githubOwnerRepoRe matches the first github.com/owner/repo path in a string.
var githubOwnerRepoRe = regexp.MustCompile(`github\.com/([^/{]+)/([^/{]+)`)

// githubTreeRefRe extracts the ref from a GitHub tree URL like
// https://github.com/owner/repo/tree/v3.0.1{/dir}
var githubTreeRefRe = regexp.MustCompile(`github\.com/[^/]+/[^/]+/tree/([^{/\s]+)`)

// parseVanityMeta extracts the GitHub owner, repo, and optional ref from the
// go-import and go-source meta tags in an HTML body.
// go-import is checked first for the canonical repo URL.
// go-source is always checked for a ref (branch/tag encoded in the tree URL),
// and also provides owner/repo for proxies like gopkg.in that point their
// go-import VCS URL at themselves rather than GitHub.
func parseVanityMeta(body []byte) (owner, repo, ref string, ok bool) {
	metaContent := func(m [][]byte) string {
		if len(m[1]) > 0 {
			return string(m[1])
		}
		return string(m[2])
	}

	if m := goImportRe.FindSubmatch(body); m != nil {
		if gh := githubOwnerRepoRe.FindStringSubmatch(metaContent(m)); gh != nil {
			owner, repo, ok = gh[1], gh[2], true
		}
	}
	if m := goSourceRe.FindSubmatch(body); m != nil {
		content := metaContent(m)
		if gh := githubOwnerRepoRe.FindStringSubmatch(content); gh != nil {
			if !ok {
				owner, repo, ok = gh[1], gh[2], true
			}
			if rm := githubTreeRefRe.FindStringSubmatch(content); rm != nil {
				ref = rm[1]
			}
		}
	}
	return
}

// resolveVanityModule fetches the go-import/go-source meta tags for a vanity
// module path and returns the GitHub owner, repo, and optional ref.
func resolveVanityModule(client *http.Client, module string) (owner, repo, ref string, ok bool) {
	resp, err := client.Get("https://" + module + "?go-get=1")
	if err != nil {
		return
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}
	owner, repo, ref, ok = parseVanityMeta(body)
	return
}

func moduleToGitHubPath(module string) (owner, repo string, ok bool) {
	// Direct well-known remappings (exact match)
	if gh, found := wellKnownModules[module]; found {
		// gh is a full github.com/owner/repo path
		parts := strings.SplitN(strings.TrimPrefix(gh, "github.com/"), "/", 2)
		if len(parts) == 2 {
			return parts[0], parts[1], true
		}
	}

	// Prefix-based remappings (e.g. golang.org/x/ -> github.com/golang/)
	for prefix, ghPrefix := range wellKnownModules {
		if strings.HasPrefix(module, prefix) && strings.HasSuffix(prefix, "/") {
			remainder := strings.TrimPrefix(module, prefix)
			// Take only the first path segment as the repo name
			repoName := strings.SplitN(remainder, "/", 2)[0]
			ghOwner := strings.TrimPrefix(strings.TrimSuffix(ghPrefix, "/"), "github.com/")
			return ghOwner, repoName, true
		}
	}

	// Standard github.com/owner/repo module paths
	if rest, found := strings.CutPrefix(module, "github.com/"); found {
		parts := strings.SplitN(rest, "/", 3)
		if len(parts) >= 2 {
			return parts[0], parts[1], true
		}
	}

	return "", "", false
}

func fetchGitHubInfo(client *http.Client, owner, repo, ref, token string) (description, homepage, license string, topics, readmeLines []string, err error) {
	authHeader := ""
	if token != "" {
		authHeader = "Bearer " + token
	}

	doRequest := func(url string, out any) error {
		req, e := http.NewRequest("GET", url, nil)
		if e != nil {
			return e
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
		req.Header.Set("User-Agent", "go-mod-summary")
		if authHeader != "" {
			req.Header.Set("Authorization", authHeader)
		}
		resp, e := client.Do(req)
		if e != nil {
			return e
		}
		defer resp.Body.Close()
		if resp.StatusCode == 404 {
			return fmt.Errorf("not found")
		}
		if resp.StatusCode == 403 && resp.Header.Get("X-RateLimit-Remaining") == "0" {
			return fmt.Errorf("rate limit exceeded (set GITHUB_TOKEN for 5000 req/hour)")
		}
		if resp.StatusCode != 200 {
			return fmt.Errorf("HTTP %d", resp.StatusCode)
		}
		return json.NewDecoder(resp.Body).Decode(out)
	}

	// Fetch repo info (description/about)
	var repoInfo githubRepo
	if e := doRequest(fmt.Sprintf("https://api.github.com/repos/%s/%s", owner, repo), &repoInfo); e != nil {
		err = e
		return
	}
	description = mdEmojiRe.ReplaceAllString(repoInfo.Description, "")
	description = strings.TrimSpace(strings.ReplaceAll(description, "[mirror]", ""))
	description = strings.Join(strings.Fields(description), " ")
	homepage = strings.TrimSpace(repoInfo.Homepage)
	topics = repoInfo.Topics
	if id := repoInfo.License.SPDXId; id != "" && id != "NOASSERTION" {
		license = id
	} else if name := repoInfo.License.Name; name != "" {
		license = name
	}

	// Fetch README
	var readme githubReadme
	readmeURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/readme", owner, repo)
	if ref != "" {
		readmeURL += "?ref=" + ref
	}
	if e := doRequest(readmeURL, &readme); e != nil {
		// README missing is non-fatal
		return
	}

	var raw []byte
	if readme.Encoding == "base64" {
		// GitHub wraps lines at 60 chars with \n
		cleaned := strings.ReplaceAll(readme.Content, "\n", "")
		raw, err = base64.StdEncoding.DecodeString(cleaned)
		if err != nil {
			err = nil // non-fatal
			return
		}
	} else {
		raw = []byte(readme.Content)
	}

	readmeLines = extractReadmeLines(string(raw))
	return
}

// mdImageRe matches any Markdown image in both inline and reference styles,
// with or without a wrapping link:
//
//	![alt](url)           inline image
//	![alt][ref]           reference-style image
//	[![alt](url)](url)    inline linked image
//	[![alt][ref]][ref]    reference-style linked image (e.g. [![][badge-svg]][badge-url])
var mdImageRe = regexp.MustCompile(`\[?!\[[^\]]*\](?:\([^)]*\)|\[[^\]]*\])\]?(?:\([^)]*\)|\[[^\]]*\])?`)

// mdEmojiRe matches Markdown emoji codes like :rocket: or :check_mark:
var mdEmojiRe = regexp.MustCompile(`:[a-z][a-z0-9_]*:`)

// extractReadmeLines returns the first meaningful lines from a README,
// skipping blank lines, HTML tags, badges, and Markdown heading markers.
func extractReadmeLines(content string) []string {
	lines := strings.Split(content, "\n")
	var out []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue // skip blank lines throughout; stop condition is ## headings
		}
		// Skip HTML tags (e.g. <div>, <img>, <p>, <!-- -->)
		if strings.HasPrefix(trimmed, "<") || strings.HasPrefix(trimmed, "<!--") {
			continue
		}
		// Stop at subsection headings (## or deeper) — marks the end of the intro.
		if strings.HasPrefix(trimmed, "##") {
			break
		}
		// Strip all Markdown image/badge patterns, then skip if nothing remains.
		trimmed = mdImageRe.ReplaceAllString(trimmed, "")
		// Remove Markdown emphasis markers and "[mirror]" noise.
		trimmed = strings.ReplaceAll(trimmed, "**", "")
		trimmed = strings.ReplaceAll(trimmed, "__", "")
		trimmed = strings.ReplaceAll(trimmed, "[mirror]", "")
		// Remove Markdown emoji codes (:rocket:, :white_check_mark:, etc.)
		trimmed = mdEmojiRe.ReplaceAllString(trimmed, "")
		// Collapse runs of whitespace left by removals, then trim edges.
		trimmed = strings.Join(strings.Fields(trimmed), " ")
		if trimmed == "" {
			continue
		}
		// Skip lines that are only Markdown horizontal rules or === / ---
		if isDecorative(trimmed) {
			continue
		}
		// Strip leading # characters (Markdown headings)
		stripped := strings.TrimLeft(trimmed, "#")
		stripped = strings.TrimSpace(stripped)
		if stripped == "" {
			continue
		}
		out = append(out, stripped)
		if len(out) >= 5 {
			break
		}
	}
	return out
}

// isDecorative returns true for lines that are purely decorative (---, ===, ***).
func isDecorative(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, c := range s {
		if c != '-' && c != '=' && c != '*' && c != '_' {
			return false
		}
	}
	return true
}

// normalizeModSrc expands a shorthand -f value to a full path or URL pointing
// at a go.mod file:
//   - Already ending in "go.mod"          → unchanged.
//   - Local directory                      → filepath.Join(src, "go.mod").
//   - https:// URL not ending in "go.mod"  → appends "/blob/HEAD/go.mod";
//     fetchModContent's blob→raw rewrite handles the rest.
//   - Go module path (no scheme)           → resolved to a
//     raw.githubusercontent.com URL, using the vanity resolver if needed.
func normalizeModSrc(client *http.Client, src string) string {
	if strings.HasSuffix(src, "go.mod") {
		return src
	}
	if strings.HasPrefix(src, "https://") {
		return strings.TrimRight(src, "/") + "/blob/HEAD/go.mod"
	}
	// Local directory?
	if info, err := os.Stat(src); err == nil && info.IsDir() {
		return filepath.Join(src, "go.mod")
	}
	// Treat as a Go module path.
	owner, repo, ok := moduleToGitHubPath(src)
	ref := ""
	if !ok {
		owner, repo, ref, ok = resolveVanityModule(client, src)
	}
	if ok {
		if ref == "" {
			ref = "HEAD"
		}
		return fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/go.mod", owner, repo, ref)
	}
	return src
}

// goModCache returns the Go module cache directory ($GOMODCACHE, or the default).
func goModCache() string {
	if c := os.Getenv("GOMODCACHE"); c != "" {
		return c
	}
	if p := os.Getenv("GOPATH"); p != "" {
		return filepath.Join(p, "pkg", "mod")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "go", "pkg", "mod")
}

// readCachedReadme reads the README from the Go module cache zip for the given
// module path and version. The zip lives at:
//
//	$GOMODCACHE/cache/download/{escaped-path}/@v/{escaped-version}.zip
//
// Returns (lines, true) when the zip exists (even if there is no README), or
// (nil, false) when the module is not in the cache at all.
func readCachedReadme(cacheDir, modPath, version string) (lines []string, cached bool) {
	escapedPath, err := module.EscapePath(modPath)
	if err != nil {
		return nil, false
	}
	escapedVersion, err := module.EscapeVersion(version)
	if err != nil {
		return nil, false
	}

	zipPath := filepath.Join(cacheDir, "cache", "download",
		filepath.FromSlash(escapedPath), "@v", escapedVersion+".zip")
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, false // not cached
	}
	defer zr.Close()

	// Zip entries are named "{modPath}@{version}/{file}".
	prefix := modPath + "@" + version + "/"
	for _, f := range zr.File {
		rel := strings.TrimPrefix(f.Name, prefix)
		if strings.Contains(rel, "/") {
			continue // not a top-level file
		}
		if strings.HasPrefix(strings.ToLower(rel), "readme") {
			rc, err := f.Open()
			if err != nil {
				break
			}
			data, _ := io.ReadAll(rc)
			rc.Close()
			return extractReadmeLines(string(data)), true
		}
	}
	return nil, true // cached but no README
}

// printRepoSummary prints the GitHub summary for a single resolved repo.
// modulePath is the import path shown as the label (may differ from owner/repo for vanity paths).
// version and vanity are optional — pass empty string / false when not applicable.
func printRepoSummary(client *http.Client, modulePath, version string, owner, repo, ref string, vanity bool, token string, maxReadmeLines int) {
	desc, homepage, license, topics, lines, err := fetchGitHubInfo(client, owner, repo, ref, token)
	if err != nil {
		fmt.Printf("%s  error: %v\n\n", modulePath, err)
		return
	}

	fmt.Printf("%s\n", modulePath)
	if version != "" {
		fmt.Printf("  Version: %s\n", version)
	}
	if vanity {
		ghURL := fmt.Sprintf("https://github.com/%s/%s", owner, repo)
		if ref != "" {
			ghURL += "/tree/" + ref
		}
		fmt.Printf("  GitHub:  %s\n", ghURL)
	}
	if desc != "" {
		fmt.Printf("  About:   %s\n", desc)
	}
	if license != "" {
		fmt.Printf("  License: %s\n", license)
	}
	if homepage != "" {
		fmt.Printf("  Website: %s\n", homepage)
	}
	if len(topics) > 0 {
		fmt.Printf("  Topics:  %s\n", strings.Join(topics, ", "))
	}
	limit := min(maxReadmeLines, len(lines))
	for i, line := range lines[:limit] {
		prefix := "  README: "
		if i > 0 {
			prefix = "          "
		}
		fmt.Printf("%s %s\n", prefix, line)
	}
	fmt.Println()
}

// isLocalPath reports whether src refers to a local filesystem path rather
// than a URL or Go module path.
func isLocalPath(src string) bool {
	if strings.HasPrefix(src, ".") || strings.HasPrefix(src, "/") {
		return true
	}
	if strings.HasPrefix(src, "https://") {
		return false
	}
	info, err := os.Stat(src)
	return err == nil && info.IsDir()
}

// resolveForSummary resolves src to a GitHub owner/repo for the -s flag.
// For local paths it reads the go.mod to discover the module path first.
func resolveForSummary(client *http.Client, src string) (modulePath, owner, repo, ref string, vanity, ok bool) {
	if isLocalPath(src) {
		modFile := src
		if info, err := os.Stat(src); err == nil && info.IsDir() {
			modFile = filepath.Join(src, "go.mod")
		}
		data, err := os.ReadFile(modFile)
		if err != nil {
			return
		}
		f, err := modfile.Parse(modFile, data, nil)
		if err != nil {
			return
		}
		src = f.Module.Mod.Path
	}
	modulePath = src
	// Strip https:// so moduleToGitHubPath can handle github.com URLs too.
	modPath := strings.TrimPrefix(src, "https://")
	owner, repo, ok = moduleToGitHubPath(modPath)
	if !ok {
		owner, repo, ref, ok = resolveVanityModule(client, modPath)
		vanity = ok
	}
	return
}

func main() {
	showSummary := flag.Bool("s", false, "show a summary of the top-level module/repo (About, Website, Topics, README)")
	showMod := flag.Bool("m", false, "show go.mod dependency summaries")
	includeIndirect := flag.Bool("i", false, "include indirect dependencies (with -m)")
	noCache := flag.Bool("no-cache", false, "skip the local module cache and always fetch from GitHub")
	noGitHub := flag.Bool("no-github", false, "never call GitHub; only use the local module cache (cache misses are skipped)")
	readmeLines := flag.Int("lines", 3, "number of README lines to show (0 to disable)")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: go-mod-summary [flags] [directory|URL|module]\n\n")
		fmt.Fprintf(os.Stderr, "Summarizes a Go module and/or its dependencies by fetching descriptions,\nhomepages, topics, and README excerpts from GitHub.\n\n")
		fmt.Fprintf(os.Stderr, "The argument can be a local directory, a go.mod file path, a GitHub URL,\nor a Go module path (github.com/… or vanity domain). Defaults to \".\".\n\n")
		fmt.Fprintf(os.Stderr, "If neither -s nor -m is given, both are enabled.\n\n")
		fmt.Fprintf(os.Stderr, "By default, the local Go module cache is checked first for each dependency's\nREADME; on a cache miss the GitHub API is used. Use -no-cache or -no-github\nto override.\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nEnvironment:\n")
		fmt.Fprintf(os.Stderr, "  GITHUB_TOKEN  Personal access token for GitHub API (raises limit from 60 to 5000 req/hour)\n")
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  go-mod-summary .                                    # full summary of current directory\n")
		fmt.Fprintf(os.Stderr, "  go-mod-summary -s github.com/spf13/cobra            # repo summary only\n")
		fmt.Fprintf(os.Stderr, "  go-mod-summary -s gopkg.in/yaml.v3                  # vanity path summary\n")
		fmt.Fprintf(os.Stderr, "  go-mod-summary -m .                                 # go.mod deps only\n")
		fmt.Fprintf(os.Stderr, "  go-mod-summary -m -i .                              # include indirect deps\n")
		fmt.Fprintf(os.Stderr, "  go-mod-summary https://github.com/owner/repo        # remote repo\n")
		fmt.Fprintf(os.Stderr, "  go-mod-summary https://github.com/owner/repo/blob/main/go.mod\n")
		fmt.Fprintf(os.Stderr, "  go-mod-summary -m -no-github .                      # offline, cache only\n")
		fmt.Fprintf(os.Stderr, "  go-mod-summary -m -no-cache .                       # always fetch from GitHub\n")
	}

	flag.Parse()
	args := flag.Args()

	// No arguments at all → print help.
	if len(args) == 0 && !*showSummary && !*showMod {
		flag.Usage()
		os.Exit(0)
	}

	src := "."
	if len(args) > 0 {
		src = args[0]
	}

	// If neither mode flag is set, enable both.
	if !*showSummary && !*showMod {
		*showSummary = true
		*showMod = true
	}

	token := os.Getenv("GITHUB_TOKEN")
	client := &http.Client{}

	if *showSummary {
		modulePath, owner, repo, ref, vanity, ok := resolveForSummary(client, src)
		if !ok {
			fmt.Fprintf(os.Stderr, "error: cannot resolve %q to a GitHub repository\n", src)
			os.Exit(1)
		}
		printRepoSummary(client, modulePath, "", owner, repo, ref, vanity, token, *readmeLines)
	}

	if *showMod {
		data, label, err := fetchModContent(client, normalizeModSrc(client, src))
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading go.mod from %s: %v\n", src, err)
			os.Exit(1)
		}
		f, err := modfile.Parse(label, data, nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error parsing go.mod: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Module: %s\n\n", f.Module.Mod.Path)

		cacheDir := goModCache()
		for _, req := range f.Require {
			if req.Indirect && !*includeIndirect {
				continue
			}
			depLabel := req.Mod.Path
			if req.Indirect {
				depLabel += " (indirect)"
			}

			// Try the local module cache first (unless -no-cache).
			if !*noCache {
				if lines, cached := readCachedReadme(cacheDir, req.Mod.Path, req.Mod.Version); cached {
					fmt.Printf("%s\n", depLabel)
					fmt.Printf("  Version: %s\n", req.Mod.Version)
					limit := min(*readmeLines, len(lines))
					for i, line := range lines[:limit] {
						prefix := "  README: "
						if i > 0 {
							prefix = "          "
						}
						fmt.Printf("%s %s\n", prefix, line)
					}
					fmt.Println()
					continue
				}
			}

			// Cache miss (or -no-cache): fall back to GitHub unless -no-github.
			if *noGitHub {
				fmt.Printf("%-55s  (not in module cache)\n\n", req.Mod.Path)
				continue
			}

			owner, repo, ok := moduleToGitHubPath(req.Mod.Path)
			var ref string
			vanity := false
			if !ok {
				owner, repo, ref, ok = resolveVanityModule(client, req.Mod.Path)
				vanity = ok
			}
			if !ok {
				fmt.Printf("%-55s  (no GitHub mapping)\n\n", req.Mod.Path)
				continue
			}
			printRepoSummary(client, depLabel, req.Mod.Version, owner, repo, ref, vanity, token, *readmeLines)
		}
	}
}
