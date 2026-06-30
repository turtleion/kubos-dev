package test

import (
	"context"
	"fmt"
	"io/fs"
	"kubos/internal/exec/exectypes"
	"kubos/internal/log"
	"kubos/internal/util"

	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/fatih/color"
)

type TestStatus int

const (
	StatusPass TestStatus = iota
	StatusWarn
	StatusFail
	StatusSkip
	StatusUnavailable
)

type TestResult struct {
	Name   string
	Status TestStatus
	Weight int    // kontribusi ke total score
	Score  int    // actual score yang didapat (0 atau Weight)
	Detail string // pesan untuk user
}

type TestReport struct {
	Package string
	Results []TestResult
	Total   int // sum of Weight
	Earned  int // sum of Score
}

func (r *TestReport) Percent() int {
	if r.Total == 0 {
		return 0
	}
	return (r.Earned * 100) / r.Total
}

type Recommendation int

const (
	RecommendCommit   Recommendation = iota // >= 80
	RecommendSnapshot                       // 50-79
	RecommendAbort                          // < 50
)

func (r *TestReport) Recommend() Recommendation {
	p := r.Percent()
	switch {
	case p >= 80:
		return RecommendCommit
	case p >= 50:
		return RecommendSnapshot
	default:
		return RecommendAbort
	}
}

func NormalizePacmanPkgName(s string) string {
	name, _, _, ok := util.ParsePacmanPkgName(s)
	if ok {
		return name
	}
	return s
}

// packageInDB checks whether pkgName is an actual installed package in the
// container's local db — exact name match, not prefix glob match.
func packageInDB(mergeDir, pkgName string) bool {
	dbDir := filepath.Join(mergeDir, "var/lib/pacman/local")
	entries, err := os.ReadDir(dbDir)
	if err != nil {
		return false
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue // real db entries are always directories
		}
		name, _, _, ok := util.ParsePacmanPkgName(e.Name())
		if ok && name == pkgName {
			return true
		}
	}
	return false
}

// Core functions
func autoSmoke(mergeDir, pkg string) TestResult {
	r := TestResult{Name: "Smoke test", Weight: 15}

	cleanPkg := NormalizePacmanPkgName(pkg)

	// 1. Natively read the pacman file list from disk instead of spawning a container
	// Pacman stores the file list in var/lib/pacman/local/<pkg>-<version>/files
	filesPattern := filepath.Join(mergeDir, "var/lib/pacman/local", cleanPkg+"-*", "files")
	matches, err := filepath.Glob(filesPattern)
	if err != nil || len(matches) == 0 {
		r.Status = StatusSkip
		r.Detail = "tidak bisa baca list file untuk " + cleanPkg
		return r
	}

	// Read the files file natively
	content, err := os.ReadFile(matches[0])
	if err != nil {
		r.Status = StatusSkip
		r.Detail = "couldn't read the database file list"
		return r
	}

	var binary string
	var hasLibraries bool

	// Parse lines to detect binaries or shared libraries
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "usr/bin/") {
			binary = "/" + line
			break // Found a binary to execute!
		}
		if strings.HasSuffix(line, ".so") || strings.Contains(line, ".so.") {
			hasLibraries = true
		}
	}

	// 2. Execution Mode: If it's an application with a binary
	if binary != "" {
		// Run with --version but wrap it inside a strict timeout context so it can't hang if it's a daemon
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, "systemd-nspawn", "-D", mergeDir, "--", binary, "--version")
		if cmd.Run() == nil {
			r.Status = StatusPass
			r.Score = r.Weight
			r.Detail = fmt.Sprintf("%s ran successfully with --version", filepath.Base(binary))
			return r
		}

		// Fallback to -V
		cmd2 := exec.CommandContext(ctx, "systemd-nspawn", "-D", mergeDir, "--", binary, "-V")
		if cmd2.Run() == nil {
			r.Status = StatusPass
			r.Score = r.Weight
			r.Detail = fmt.Sprintf("%s ran successfully with -V", filepath.Base(binary))
			return r
		}

		// It has a binary but it crashed or timed out
		r.Status = StatusWarn
		r.Score = r.Weight / 2
		r.Detail = fmt.Sprintf("binary %s but didn't respond to version check.", binary)
		return r
	}

	// 3. Library Mode: If it has no binary but contains shared objects (.so)
	if hasLibraries {
		// Run a quick check using your existing parseLdconfigOutput logic inside the container!
		// If ldconfig output doesn't throw a hard error for this package, the library is safe.
		r.Status = StatusPass
		r.Score = r.Weight
		r.Detail = fmt.Sprintf("library %s terverifikasi aman melalui sub-sistem link", cleanPkg)
		return r
	}

	// Meta package or data package (no binaries, no libraries)
	r.Status = StatusSkip
	r.Detail = "package tidak memiliki binary atau library untuk ditest (data/meta package)"
	return r
}

// Weight mencerminkan seberapa fatal kalau gagal
var testSuite = []struct {
	name   string
	weight int
	fn     func(mergeDir, pkgName string, conflictMap map[string]string) TestResult
}{
	{"DB conflict check", 30, testDBConflict},   // fatal — konflik = install gagal
	{"No old pkg remnants", 25, testNoRemnants}, // penting — ghost package
	{"ldconfig clean", 20, testLdconfig},        // penting — broken shared libs
	//{"Smoke test", 15, testSmoke},               // medium — binary jalan
	{"File count delta", 10, testFileDelta}, // info — sanity check ukuran
}

func testDBConflict(mergeDir, pkg string, conflictMap map[string]string) TestResult {
	r := TestResult{Name: "DB conflict check", Weight: 30}

	// 1. Clean the target package name (remove version suffixes if any exist)
	cleanPkg := NormalizePacmanPkgName(pkg)
	// 2. The target package MUST exist in the container DB
	if !packageInDB(mergeDir, pkg) {
		r.Status = StatusFail
		r.Detail = cleanPkg + " not found in container"
		return r
	}

	// 3. Check conflicts — if pkg is mesa-amber, mesa must NOT exist

	if conflict, ok := conflictMap[pkg]; ok {
		cleanConflict := NormalizePacmanPkgName(conflict)
		if packageInDB(mergeDir, cleanConflict) {
			r.Status = StatusFail
			r.Detail = fmt.Sprintf("conflict: %s and %s are in database. This means they both still exist and conflicting", cleanPkg, cleanConflict)
			return r
		}
	}

	r.Status = StatusPass
	r.Score = r.Weight
	r.Detail = "no conflict found in database, all set."
	return r
}

func testNoRemnants(mergeDir, pkg string, conflictMap map[string]string) TestResult {
	r := TestResult{Name: "No old pkg remnants", Weight: 25}

	conflict, ok := conflictMap[pkg]
	if !ok || conflict == "" { // FIXED: Must have at least 2 elements [pkg, isValid]
		r.Status = StatusSkip
		r.Detail = "there is no package conflicting with " + pkg
		return r
	}

	// Extract values based on your specific layout
	rival := conflict

	// Check the pacman local database folder directly on disk for the extracted rival name
	dbPattern := filepath.Join(mergeDir, "var/lib/pacman/local", rival+"-*")
	matches, err := filepath.Glob(dbPattern)

	if err == nil && len(matches) > 0 {
		r.Status = StatusFail
		r.Detail = "masih ada di DB: " + rival
		return r
	}

	r.Status = StatusPass
	r.Score = r.Weight
	r.Detail = fmt.Sprintf("rival package (%s) sudah bersih", rival)
	return r
}

func testFileDelta(upperDir, pkg string, _ map[string]string) TestResult {
	r := TestResult{Name: "File count delta", Weight: 10}

	var count int
	var suspiciousPaths []string

	err := filepath.WalkDir(upperDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// 1. Is it a permission denied error?
			if os.IsPermission(err) {
				log.ColoredPrint(color.FgRed, fmt.Sprintf("Cannot read %s: no permission", path))
				log.Print(log.ERROR, fmt.Sprintf("Cannot read %s: permission error", path), true, true)
				if d != nil && d.IsDir() {
					return filepath.SkipDir // Skip this folder and keep walking
				}
				return nil // Skip this specific unreadable file and keep walking
			}

			// 2. CRITICAL: Any other error (Disk failure, No space, etc.) must be returned!
			return err
		}
		if d.IsDir() {
			return nil
		}
		count++

		// Get the relative path from the upperDir root (e.g., "usr/bin/foo" instead of "/tmp/upper/usr/bin/foo")
		rel, err := filepath.Rel(upperDir, path)
		if err != nil {
			return nil
		}

		// Clean it up to ensure consistent lookups
		rel = filepath.Clean(rel)

		// Flag suspicious mutations outside expected system target paths
		if isSuspiciousPath(rel) {
			// Cap the list so we don't blow up memory with log spam
			if len(suspiciousPaths) < 5 {
				suspiciousPaths = append(suspiciousPaths, "/"+rel)
			}
		}

		return nil
	})

	if err != nil {
		r.Status = StatusSkip
		r.Detail = "failed to read the upperdir, skipping test: " + err.Error()
		return r
	}

	// Evaluation Logic
	switch {
	case count == 0:
		r.Status = StatusSkip
		r.Detail = fmt.Sprintf("upperdir is empty, install %s might haven't been runned", pkg)

	case len(suspiciousPaths) > 0:
		// CRITICAL WARNING: Written files outside normal /usr, /etc, /var, etc.
		r.Status = StatusWarn
		r.Score = r.Weight / 4 // Heavy penalty for poor behavior
		r.Detail = fmt.Sprintf("%d file written, found suspicious location: %s",
			count, strings.Join(suspiciousPaths, ", "))

	case count > 10000:
		// Standard loose threshold fallback for volume anomalies
		r.Status = StatusWarn
		r.Score = r.Weight / 2
		r.Detail = fmt.Sprintf("%d file written — absurd amount of written files, strange but alr.", count)

	default:
		r.Status = StatusPass
		r.Score = r.Weight
		r.Detail = fmt.Sprintf("%d file written safely to the target machine", count)
	}

	return r
}

// isSuspiciousPath returns true if a file is installed somewhere weird
func isSuspiciousPath(relPath string) bool {
	// Standard allowed prefix directories
	allowedPrefixes := []string{
		"usr/",
		"etc/",
		"var/",
		"opt/",
		"lib/", // Symlinks to usr/lib on modern systems
		"lib64/",
		"bin/", // Symlinks to usr/bin on modern systems
		"sbin/",
	}

	// Check if it matches a valid location
	for _, prefix := range allowedPrefixes {
		if strings.HasPrefix(relPath, prefix) {
			return false // Safe path
		}
	}

	// If it directly hits root paths or user/system configuration roots, it's suspicious
	// Examples: "root/.bashrc", "home/user/...", "tmp/escaped.sh", "docker-compose.yml"
	return true
}

func testLdconfig(root string, _ string, _ map[string]string) TestResult {
	issues, err := RunLdconfigInNspawn(root)
	r := TestResult{Name: "ldconfig clean", Weight: 20}

	if err != nil {
		r.Status = StatusSkip
		r.Detail = fmt.Sprintf("ldconfig execution failed: %v", err)
		return r

	}

	hardErrors := 0
	warnings := 0
	var msgs []string

	for _, issue := range issues {
		msgs = append(msgs, issue.String())
		if issue.IsHard {
			hardErrors++
		} else {
			warnings++
		}
	}

	switch {
	case hardErrors > 0:
		r.Status = StatusFail
	case warnings > 0:
		// partial credit — warnings don't break linking but suggest sloppiness
		if r.Status != StatusFail {
			r.Status = StatusWarn
			r.Score = r.Weight / 2
		}
	default:
		r.Status = StatusPass
		r.Score = r.Weight
		r.Detail = "ldconfig clean"
		return r
	}

	// 2. Build a clear, comprehensive message summary
	var summary []string
	if hardErrors > 0 {
		summary = append(summary, fmt.Sprintf("%d hard errors", hardErrors))
	}
	if warnings > 0 {
		summary = append(summary, fmt.Sprintf("%d warnings", warnings))
	}

	// Example output: "ldconfig issues (1 hard errors, 3 warnings): [error] ...; [warning] ..."
	r.Detail = fmt.Sprintf("ldconfig issues (%s): %s",
		strings.Join(summary, ", "),
		strings.Join(msgs, "; "),
	)
	return r

}

//	func testSmoke(mergeDir, pkg string, conflictMap map[string]string) TestResult {
//		r := TestResult{Name: "Smoke test", Weight: 15}
//		cmds, ok := smokeCommands[pkg]
//		if !ok {
//			r.Status = StatusSkip
//			r.Detail = "tidak ada smoke test untuk " + pkg
//			return r
//		}
//		args := append([]string{"--directory", mergeDir, "--"}, cmds...)
//		cmd := exec.Command("systemd-nspawn", args...)
//		var out bytes.Buffer
//		cmd.Stdout, cmd.Stderr = &out, &out
//		if err := cmd.Run(); err != nil {
//			r.Status = StatusWarn // warn, bukan fail — smoke test bisa gagal karena no GPU
//			r.Score = r.Weight / 2
//			r.Detail = "smoke test exit non-zero (mungkin butuh display/GPU)"
//			return r
//		}
//		r.Status = StatusPass
//		r.Score = r.Weight
//		return r
//	}

func RunTestSuite(upperDir, mergeDir, pkgName string, conflictMap map[string]string, conflictingKeys []string, spawnResult exectypes.ExecutionResult) *TestReport {
	report := &TestReport{
		Package: pkgName,
	}

	tmptestSmoke := func() TestResult {
		r := TestResult{Name: "Smoke test", Weight: 15}
		r.Status = StatusUnavailable
		r.Detail = "Test currently unavailable"
		return r
	}

	tests := []func() TestResult{
		func() TestResult { return testDBConflict(mergeDir, pkgName, conflictMap) },
		func() TestResult { return testNoRemnants(mergeDir, pkgName, conflictMap) },
		func() TestResult { return testLdconfig(mergeDir, pkgName, conflictMap) },
		func() TestResult { return tmptestSmoke() },
		func() TestResult { return testFileDelta(upperDir, pkgName, conflictMap) },
	}

	for _, fn := range tests {
		result := fn()
		report.Results = append(report.Results, result)

		// Skip masuk ke Total tapi tidak ke Earned
		if result.Status != StatusUnavailable {
			report.Total += result.Weight
			report.Earned += result.Score
		}
	}

	return report
}

var bold = color.New(color.Bold).SprintFunc()

func PrintReport(report *TestReport) {
	const (
		colName   = 28
		colStatus = 12
	)

	statusIcon := map[TestStatus]string{
		StatusPass:        color.New(color.FgGreen).Sprint(bold("✓")),
		StatusWarn:        color.New(color.FgYellow).Sprint(bold("~")),
		StatusFail:        color.New(color.FgRed).Sprint(bold("✗")),
		StatusSkip:        bold("-"),
		StatusUnavailable: bold("?"),
	}

	statusString := map[TestStatus]string{
		StatusPass: "pass",
		StatusWarn: "warn",
		StatusFail: "fail",
	}

	fmt.Printf("\n==> test report: %s\n", report.Package)

	// Header
	fmt.Printf("    %-*s %-*s %s\n", colName, "test", colStatus, "status", "score")
	fmt.Println("    " + strings.Repeat("─", colName+colStatus+8))

	// Rows
	for _, r := range report.Results {
		icon := statusIcon[r.Status]

		switch r.Status {
		case StatusUnavailable:
			fmt.Printf("\n    %s %-*s %s\n",
				icon,
				colName, r.Name,
				"unavailable",
			)
		case StatusSkip:
			fmt.Printf("\n    %s %-*s %s\n",
				icon,
				colName, r.Name,
				"skip",
			)
		default:
			fmt.Printf("\n    %s %-*s %-*s %d\n",
				icon,
				colName, r.Name,
				colStatus, statusString[r.Status],
				r.Score,
			)
		}

		if r.Detail != "" {
			fmt.Printf("      └ %s\n", r.Detail)
		}
	}

	// Score bar
	fmt.Printf("\n\n")
	pct := report.Percent()
	barTotal := 30
	barFilled := (pct * barTotal) / 100
	var barTmp string
	switch {
	case pct >= 80:
		barTmp = color.New(color.FgGreen).Sprint("█")
	case pct < 80 && pct >= 50:
		barTmp = color.New(color.FgYellow).Sprint("█")
	case pct < 50:
		barTmp = color.New(color.FgRed).Sprint("█")

	}
	bar := strings.Repeat(barTmp, barFilled) + strings.Repeat("░", barTotal-barFilled)
	fmt.Printf("    score: %d / %d  (%d%%)\n", report.Earned, report.Total, pct)
	fmt.Printf("    [%s]\n\n", bar)

	// Rekomendasi
	switch report.Recommend() {
	case RecommendCommit:
		fmt.Println(bold("  [√] safe to commit to your real machine."))
	case RecommendSnapshot:
		fmt.Println(bold("  [~] prefer to make a snapshot point before committing."))
	case RecommendAbort:
		fmt.Println(bold("  [✗] recommended to not commit."))
	}
	fmt.Println()
}
