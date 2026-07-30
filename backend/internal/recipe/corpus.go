package recipe

import (
	"encoding/json"
	"os"
	"time"
)

// SourceInfo represents metadata stored in SOURCE.json for the howtocook corpus.
type SourceInfo struct {
	UpstreamURL     string    `json:"upstream_url"`
	UpstreamCommit  string    `json:"upstream_commit"`
	CopiedAt        time.Time `json:"copied_at"`
	MarkdownCount   int       `json:"markdown_count"`
	ResourceCount   int       `json:"resource_count"`
	DirectorySHA256 string    `json:"directory_sha256"`
}

// ReadSourceJSON reads and parses SOURCE.json under the given resources directory.
func ReadSourceJSON(resourcesDir string) (*SourceInfo, error) {
	fpath := resourcesDir + string(os.PathSeparator) + "SOURCE.json"
	f, err := os.Open(fpath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var info SourceInfo
	if err := json.NewDecoder(f).Decode(&info); err != nil {
		return nil, err
	}
	return &info, nil
}
