package ragregression

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

var shaPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

type Case struct {
	CaseID         string   `json:"case_id"`
	Query          string   `json:"query"`
	ExpectedAnswer string   `json:"expected_answer"`
	EvidenceHashes []string `json:"evidence_hashes"`
	Tags           []string `json:"tags"`
	CaseSHA256     string   `json:"case_sha256"`
}

type Manifest struct {
	SchemaVersion int    `json:"schema_version"`
	Dataset       string `json:"dataset"`
	Version       int    `json:"version"`
	CaseCount     int    `json:"case_count"`
	FileSHA256    string `json:"file_sha256"`
}

func canonicalHash(item Case) (string, error) {
	sort.Strings(item.EvidenceHashes)
	sort.Strings(item.Tags)
	encoded, err := json.Marshal(struct {
		CaseID         string   `json:"case_id"`
		Query          string   `json:"query"`
		ExpectedAnswer string   `json:"expected_answer"`
		EvidenceHashes []string `json:"evidence_hashes"`
		Tags           []string `json:"tags"`
	}{item.CaseID, item.Query, item.ExpectedAnswer, item.EvidenceHashes, item.Tags})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func Validate(datasetPath, manifestPath string) error {
	raw, err := os.ReadFile(datasetPath)
	if err != nil {
		return err
	}
	manifestRaw, err := os.ReadFile(manifestPath)
	if err != nil {
		return err
	}
	var manifest Manifest
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		return fmt.Errorf("manifest: %w", err)
	}
	if manifest.SchemaVersion != 1 || manifest.Dataset == "" || manifest.Version < 1 {
		return fmt.Errorf("invalid manifest identity")
	}
	digest := sha256.Sum256(raw)
	if hex.EncodeToString(digest[:]) != manifest.FileSHA256 {
		return fmt.Errorf("dataset sha256 mismatch")
	}
	scanner := bufio.NewScanner(strings.NewReader(string(raw)))
	scanner.Buffer(make([]byte, 4096), 1<<20)
	seen := map[string]bool{}
	count := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		count++
		var item Case
		if err := json.Unmarshal([]byte(line), &item); err != nil {
			return fmt.Errorf("case line %d: %w", count, err)
		}
		if item.CaseID == "" || seen[item.CaseID] || item.Query == "" || item.ExpectedAnswer == "" || len(item.EvidenceHashes) == 0 {
			return fmt.Errorf("invalid case line %d", count)
		}
		seen[item.CaseID] = true
		for _, hash := range item.EvidenceHashes {
			if !shaPattern.MatchString(hash) {
				return fmt.Errorf("invalid evidence hash line %d", count)
			}
		}
		want, err := canonicalHash(item)
		if err != nil {
			return err
		}
		if want != item.CaseSHA256 {
			return fmt.Errorf("case sha256 mismatch line %d", count)
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if count != manifest.CaseCount {
		return fmt.Errorf("case count %d != %d", count, manifest.CaseCount)
	}
	return nil
}
