package cmd

import (
	"bytes"
	"fmt"
	"io/fs"
	"kubos/libraries/essentials"
	"os/exec"
	"path/filepath"
	"strings"
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
	fn     func(mergeDir, pkgName string, conflictMap map[string][]string) TestResult
}{
	{"DB conflict check", 30, testDBConflict},   // fatal — konflik = install gagal
	{"No old pkg remnants", 25, testNoRemnants}, // penting — ghost package
	{"ldconfig clean", 20, testLdconfig},        // penting — broken shared libs
	//{"Smoke test", 15, testSmoke},               // medium — binary jalan
	{"File count delta", 10, testFileDelta}, // info — sanity check ukuran
}

func testDBConflict(mergeDir, pkg string, conflictMap map[string][]string) TestResult {
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
		rival, _ := conflict[0], conflict[1]
		rival, _, _, _ = essentials.ParsePacmanPkgName(rival)
		checkRival := exec.Command("sudo", "systemd-nspawn", "--directory", mergeDir,
			"--", "pacman", "-Qi", rival)
		checkRival.Stdout, checkRival.Stderr = nil, nil
		if checkRival.Run() == nil {
			r.Status = StatusFail
			r.Detail = fmt.Sprintf("konflik: %s dan %s sama-sama ada", pkg, rival)
			return r
		}
	}

	r.Status = StatusPass
	r.Score = r.Weight
	r.Detail = "tidak ada konflik ditemukan"
	return r
}

func testNoRemnants(mergeDir, pkg string, conflictMap map[string][]string) TestResult {
	r := TestResult{Name: "No old pkg remnants", Weight: 25}

	conflict, ok := conflictMap[pkg]
	if !ok {
		// tidak ada konflik yang diketahui → skip, bukan fail
		r.Status = StatusSkip
		r.Detail = "tidak ada conflict map untuk " + pkg
		return r
	}
	rival := conflict[0]
	var found string
	cmd := exec.Command("sudo", "systemd-nspawn", "--directory", mergeDir,
		"--", "pacman", "-Qi", rival)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if cmd.Run() == nil {
		// exit 0 = package masih ada = remnant
		found = rival
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

func testFileDelta(mergeDir, pkg string, _ map[string][]string) TestResult {
	r := TestResult{Name: "File count delta", Weight: 10}

	// Hitung file di upperdir — OverlayFS hanya tulis file yang berubah ke sini
	var count int
	err := filepath.WalkDir(mergeDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			count++
		}
		return nil
	})
	if err != nil {
		r.Status = StatusFail
		r.Detail = "gagal baca upperdir: " + err.Error()
		return r
	}

	switch {
	case count == 0:
		// Install tidak menulis apa-apa ke upperdir — kemungkinan gagal diam-diam
		r.Status = StatusFail
		r.Detail = "upperdir kosong, install mungkin tidak berjalan"

	case count > 5000:
		// Terlalu banyak file — curiga ada yang salah (misal nulis ke / tanpa sandbox)
		r.Status = StatusWarn
		r.Score = r.Weight / 2
		r.Detail = fmt.Sprintf("%d file — lebih banyak dari yang diharapkan", count)

	default:
		r.Status = StatusPass
		r.Score = r.Weight
		r.Detail = fmt.Sprintf("%d file ditulis ke upperdir", count)
	}

	return r
}

func testLdconfig(mergeDir, _ string, conflictMap map[string][]string) TestResult {
	r := TestResult{Name: "ldconfig clean", Weight: 20}
	var stderr bytes.Buffer
	cmd := exec.Command("sudo", "systemd-nspawn", "--directory", mergeDir,
		"--", "ldconfig")
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		r.Status = StatusFail
		r.Detail = "ldconfig error: " + stderr.String()
		return r
	}
	// ldconfig prints warnings to stderr even on partial success
	if stderr.Len() > 0 {
		r.Status = StatusWarn
		r.Score = r.Weight / 2
		r.Detail = "ldconfig selesai dengan warning"
		return r
	}
	r.Status = StatusPass
	r.Score = r.Weight
	return r
}

//	func testSmoke(mergeDir, pkg string, conflictMap map[string][]string) TestResult {
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
func RunTestSuite(mergeDir, pkgName string, conflictMap map[string][]string, conflictingKeys []string, spawnResult essentials.ExecutionResult) *TestReport {
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
