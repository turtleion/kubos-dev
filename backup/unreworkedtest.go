package cmd

import (
	"bytes"
	"fmt"
	"io/fs"
	"kubos/libraries/essentials"
	"kubos/libraries/logger"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
)

type TestStatus int

const (
	StatusPass TestStatus = iota
	StatusWarn
	StatusFail
	StatusSkip
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
	name, _, _, ok := essentials.ParsePacmanPkgName(s)
	if ok {
		return name
	}
	return s
}

func autoSmoke(mergeDir, pkg string) TestResult {
	r := TestResult{Name: "Smoke test", Weight: 15}

	// Tanya pacman: file apa saja yang diinstall package ini?
	cmd := exec.Command("systemd-nspawn", "--directory", mergeDir,
		"--", "pacman", "-Ql", pkg)
	var out bytes.Buffer
	cmd.Stdout = &out
	if cmd.Run() != nil {
		r.Status = StatusSkip
		r.Detail = "tidak bisa list files dari " + pkg
		return r
	}

	// Cari file di /usr/bin/ → itu binary yang bisa dicoba
	var binary string
	for _, line := range strings.Split(out.String(), "\n") {
		// format: "pkgname /usr/bin/something"
		parts := strings.Fields(line)
		if len(parts) == 2 && strings.HasPrefix(parts[1], "/usr/bin/") {
			binary = parts[1]
			break
		}
	}

	if binary == "" {
		r.Status = StatusSkip
		r.Detail = "tidak ada binary di /usr/bin, skip smoke test"
		return r
	}

	// Coba jalankan binary dengan --version
	testCmd := exec.Command("systemd-nspawn", "--directory", mergeDir,
		"--", binary, "--version")
	testCmd.Stdout, testCmd.Stderr = nil, nil
	if testCmd.Run() != nil {
		// Coba -V kalau --version tidak jalan
		testCmd2 := exec.Command("systemd-nspawn", "--directory", mergeDir,
			"--", binary, "-V")
		testCmd2.Stdout, testCmd2.Stderr = nil, nil
		if testCmd2.Run() != nil {
			r.Status = StatusWarn
			r.Score = r.Weight / 2
			r.Detail = binary + " ditemukan tapi --version exit non-zero"
			return r
		}
	}

	r.Status = StatusPass
	r.Score = r.Weight
	r.Detail = "binary " + binary + " berjalan normal"
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

	// pkg target harus ADA
	checkPkg := exec.Command("sudo", "systemd-nspawn", "--directory", mergeDir,
		"--", "pacman", "-Qi", pkg)
	checkPkg.Stdout, checkPkg.Stderr = nil, nil
	if checkPkg.Run() != nil {
		r.Status = StatusFail
		r.Detail = pkg + " tidak ditemukan di database container"
		return r
	}

	// Cek konflik — kalau pkg adalah mesa-amber, mesa tidak boleh ada
	if conflict, ok := conflictMap[pkg]; ok {
		conflict, _, _, _ = essentials.ParsePacmanPkgName(conflict)
		checkconflict := exec.Command("sudo", "systemd-nspawn", "--directory", mergeDir,
			"--", "pacman", "-Qi", conflict)
		checkconflict.Stdout, checkconflict.Stderr = nil, nil
		if checkconflict.Run() == nil {
			r.Status = StatusFail
			r.Detail = fmt.Sprintf("konflik: %s dan %s sama-sama ada", pkg, conflict)
			return r
		}
	}

	r.Status = StatusPass
	r.Score = r.Weight
	r.Detail = "tidak ada konflik ditemukan"
	return r
}

func TestDBConflict(mergeDir, pkg string, conflictMap map[string]string) TestResult {
	r := TestResult{Name: "DB conflict check", Weight: 30}

	// 1. Clean the target package name (remove version suffixes if any exist)
	cleanPkg := NormalizePacmanPkgName(pkg)
	// 2. The target package MUST exist in the container DB
	pkgPattern := filepath.Join(mergeDir, "var/lib/pacman/local", cleanPkg+"-*")
	pkgMatches, err := filepath.Glob(pkgPattern)
	if err != nil || len(pkgMatches) == 0 {
		r.Status = StatusFail
		r.Detail = cleanPkg + " not found in container"
		return r
	}

	// 3. Check conflicts — if pkg is mesa-amber, mesa must NOT exist

	if conflict, ok := conflictMap[pkg]; ok {
		cleanConflict := NormalizePacmanPkgName(conflict)
		conflictPattern := filepath.Join(mergeDir, "var/lib/pacman/local", cleanConflict+"-*")
		conflictMatches, err := filepath.Glob(conflictPattern)

		if err == nil && len(conflictMatches) > 0 {
			r.Status = StatusFail
			r.Detail = fmt.Sprintf("conflict: %s and %s are in database. This mean they both still exist and conflicting", cleanPkg, cleanConflict)
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
	if !ok {
		// tidak ada konflik yang diketahui → skip, bukan fail
		r.Status = StatusSkip
		r.Detail = "there is no conflicting package with " + pkg
		return r
	}
	var found string
	cmd := exec.Command("sudo", "systemd-nspawn", "-D", mergeDir,
		"--", "pacman", "-Qi", conflict)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if cmd.Run() == nil {
		// exit 0 = package masih ada = remnant
		found = conflict
	}

	if len(found) > 0 {
		r.Status = StatusFail
		r.Detail = "masih ada di DB: " + found
		return r
	}

	r.Status = StatusPass
	r.Score = r.Weight
	r.Detail = "semua rival package sudah bersih"
	return r
}

func TestNoRemnants(mergeDir, pkg string, conflictMap map[string]string) TestResult {
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

// func testFileDelta(upperDir, pkg string, _ map[string][]string) TestResult {
// 	r := TestResult{Name: "File count delta", Weight: 10}

// 	// Hitung file di upperdir — OverlayFS hanya tulis file yang berubah ke sini
// 	var count int
// 	err := filepath.WalkDir(upperDir, func(path string, d fs.DirEntry, err error) error {
// 		if err != nil {
// 			return err
// 		}
// 		if !d.IsDir() {
// 			count++
// 		}
// 		return nil
// 	})
// 	if err != nil {
// 		r.Status = StatusFail
// 		r.Detail = "gagal baca upperdir: " + err.Error()
// 		return r
// 	}

// 	switch {
// 	case count == 0:
// 		// Install tidak menulis apa-apa ke upperdir — kemungkinan gagal diam-diam
// 		r.Status = StatusFail
// 		r.Detail = "upperdir kosong, install mungkin tidak berjalan"

// 	case count > 5000:
// 		// Terlalu banyak file — curiga ada yang salah (misal nulis ke / tanpa sandbox)
// 		r.Status = StatusWarn
// 		r.Score = r.Weight / 2
// 		r.Detail = fmt.Sprintf("%d file — lebih banyak dari yang diharapkan", count)

// 	default:
// 		r.Status = StatusPass
// 		r.Score = r.Weight
// 		r.Detail = fmt.Sprintf("%d file ditulis ke upperdir", count)
// 	}

// 	return r
// }

func testFileDelta(upperDir, pkg string, _ map[string]string) TestResult {
	r := TestResult{Name: "File count delta", Weight: 10}

	var count int
	var suspiciousPaths []string

	err := filepath.WalkDir(upperDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// 1. Is it a permission denied error?
			if os.IsPermission(err) {
				logger.ColoredPrint(color.FgRed, fmt.Sprintf("Cannot read %s: no permission", path))
				logger.Print(essentials.LOG_ERROR, fmt.Sprintf("Cannot read %s: permission error", path), true, true)
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

// func testLdconfig(mergeDir, _ string, conflictMap map[string]string) TestResult {
// 	r := TestResult{Name: "ldconfig clean", Weight: 20}
// 	var stderr bytes.Buffer
// 	cmd := exec.Command("sudo", "systemd-nspawn", "--directory", mergeDir,
// 		"--", "ldconfig")
// 	cmd.Stderr = &stderr
// 	if err := cmd.Run(); err != nil {
// 		r.Status = StatusFail
// 		r.Detail = "ldconfig error: " + stderr.String()
// 		return r
// 	}
// 	// ldconfig prints warnings to stderr even on partial success
// 	if stderr.Len() > 0 {
// 		r.Status = StatusWarn
// 		r.Score = r.Weight / 2
// 		r.Detail = "ldconfig selesai dengan warning"
// 		return r
// 	}
// 	r.Status = StatusPass
// 	r.Score = r.Weight
// 	return r
// }

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
func RunTestSuite(mergeDir, pkgName string, conflictMap map[string]string, conflictingKeys []string, spawnResult essentials.ExecutionResult) *TestReport {
	report := &TestReport{
		Package: pkgName,
	}

	tests := []func() TestResult{
		func() TestResult { return testDBConflict(mergeDir, pkgName, conflictMap) },
		func() TestResult { return testNoRemnants(mergeDir, pkgName, conflictMap) },
		func() TestResult { return testLdconfig(mergeDir, pkgName, conflictMap) },
		// func() TestResult { return testSmoke(mergeDir, pkgName) },
		func() TestResult { return testFileDelta(mergeDir, pkgName, conflictMap) },
	}

	for _, fn := range tests {
		result := fn()
		report.Results = append(report.Results, result)

		// Skip masuk ke Total tapi tidak ke Earned
		if result.Status != StatusSkip {
			report.Total += result.Weight
			report.Earned += result.Score
		}
	}

	return report
}
