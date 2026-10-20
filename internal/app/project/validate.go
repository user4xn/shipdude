package project

import (
	"fmt"
	"regexp"
	"strings"

	"shipdude/pkg/util"
)

// Batas panjang mengikuti praktik Docker/GitHub supaya tidak ada input absurd
// yang lolos hanya karena karakternya valid.
const (
	maxRepoRefLen  = 140
	maxImageRefLen = 255
	maxGitRefLen   = 255
	maxDomainLen   = 253
)

var (
	// repoRefPattern: "owner/repo" — hanya huruf, angka, titik, garis bawah, strip.
	repoRefPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,99}/[A-Za-z0-9._-]{1,100}$`)

	// imageRefPattern: referensi image registry yang aman untuk shell
	// (huruf kecil, angka, titik, garis bawah, garis miring, strip) dengan tag opsional.
	imageRefPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._/-]{0,253}(:[A-Za-z0-9._-]{1,128})?$`)

	// gitRefPattern: nama branch/tag.
	gitRefPattern = regexp.MustCompile(`^[A-Za-z0-9._/-]{1,255}$`)

	// shaPattern: commit SHA pendek atau penuh.
	shaPattern = regexp.MustCompile(`^[a-f0-9]{7,40}$`)

	// domainPattern: hostname RFC 1123 tanpa wildcard, tanpa port, tanpa path.
	domainPattern = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$`)
)

// rejectTraversal menolak segmen ".." pada nilai yang akan dipakai sebagai path
// atau referensi: karakter valid belum tentu aman secara semantik.
func rejectTraversal(value string) bool {
	if strings.Contains(value, "..") {
		return true
	}
	return strings.HasPrefix(value, "/") || strings.HasSuffix(value, "/")
}

// ValidateRepoRef memastikan nilai berbentuk "owner/repo" dan tidak memuat
// karakter yang bisa memecah perintah shell atau path.
func ValidateRepoRef(ref string) error {
	if ref == "" {
		return util.NewBadRequestError("Repository is required", nil)
	}
	if len(ref) > maxRepoRefLen {
		return util.NewBadRequestError("Repository name is too long", nil)
	}
	if rejectTraversal(ref) || !repoRefPattern.MatchString(ref) {
		return util.NewBadRequestError(
			fmt.Sprintf("Invalid repository reference %q; expected owner/repo", ref), nil)
	}
	return nil
}

// ValidateImageRef memastikan referensi image aman dipakai sebagai argumen
// perintah, termasuk jalur rollback yang menerima nilai dari request body.
func ValidateImageRef(ref string) error {
	if ref == "" {
		return util.NewBadRequestError("Target image is required", nil)
	}
	if len(ref) > maxImageRefLen {
		return util.NewBadRequestError("Target image reference is too long", nil)
	}
	if rejectTraversal(ref) || !imageRefPattern.MatchString(ref) {
		return util.NewBadRequestError(
			fmt.Sprintf("Invalid image reference %q", ref), nil)
	}
	return nil
}

// ValidateGitRef memastikan nama branch/tag aman dipakai di URL GitHub API dan
// di dalam template workflow YAML.
func ValidateGitRef(ref string) error {
	if ref == "" {
		return util.NewBadRequestError("Branch is required", nil)
	}
	if len(ref) > maxGitRefLen {
		return util.NewBadRequestError("Branch name is too long", nil)
	}
	if rejectTraversal(ref) || !gitRefPattern.MatchString(ref) ||
		strings.HasPrefix(ref, "-") || strings.HasPrefix(ref, ".") {
		return util.NewBadRequestError(fmt.Sprintf("Invalid branch name %q", ref), nil)
	}
	return nil
}

// ValidateSHA menerima commit SHA pendek/penuh, atau "latest" untuk deploy
// image terakhir.
func ValidateSHA(sha string) error {
	if sha == "latest" {
		return nil
	}
	if !shaPattern.MatchString(sha) {
		return util.NewBadRequestError(fmt.Sprintf("Invalid commit SHA %q", sha), nil)
	}
	return nil
}

// ValidateDomain memastikan domain yang akan ditulis ke Caddyfile benar-benar
// hostname, sehingga tidak bisa menyuntikkan blok konfigurasi ingress.
func ValidateDomain(domain string) error {
	if domain == "" {
		return nil // opsional: kosong berarti pakai sslip.io
	}
	if len(domain) > maxDomainLen {
		return util.NewBadRequestError("Domain is too long", nil)
	}
	if !domainPattern.MatchString(strings.ToLower(domain)) {
		return util.NewBadRequestError(fmt.Sprintf("Invalid domain %q", domain), nil)
	}
	return nil
}

// ValidateDeployRequest menjalankan seluruh pemeriksaan sebelum job deploy
// dibuat, supaya input buruk ditolak sebagai 400 dan tidak pernah menyentuh SSH.
func ValidateDeployRequest(req DeployRequest) error {
	if err := ValidateRepoRef(req.GitHubRepo); err != nil {
		return err
	}
	if err := ValidateGitRef(req.GitHubBranch); err != nil {
		return err
	}
	if err := ValidateDomain(req.Domain); err != nil {
		return err
	}
	if strings.TrimSpace(req.ProjectName) == "" {
		return util.NewBadRequestError("Project name is required", nil)
	}
	if req.AppPort < 0 || req.AppPort > 65535 {
		return util.NewBadRequestError("Application port must be between 1 and 65535", nil)
	}
	return nil
}
