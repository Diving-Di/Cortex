package recipe

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"diary-listener/backend/internal/store"
)

// SyncResult summarizes a sync run.
type SyncResult struct {
	Scanned     int
	Created     int
	Updated     int
	Deactivated int
	Failed      int
}

// SyncCorpus performs an incremental scan of resources/howtocook and processes markdown files.
// This is a skeleton implementation: it parses files, computes content SHA256, and returns counts.
func SyncCorpus(ctx context.Context, s *store.Store, resourcesDir string) (*SyncResult, error) {
	start := time.Now()
	res := &SyncResult{}
	var runID int64
	sourceRevision := ""
	if source, err := ReadSourceJSON(resourcesDir); err == nil {
		sourceRevision = source.UpstreamCommit
	}
	if id, err := s.CreateRecipeSyncRun(ctx, sourceRevision); err == nil {
		runID = id
	}

	dishesDir := filepath.Join(resourcesDir, "dishes")
	tipsDir := filepath.Join(resourcesDir, "tips")

	// ensure directories exist
	if _, err := os.Stat(resourcesDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("resources dir missing: %s", resourcesDir)
	}

	processDir := func(base string) error {
		return filepath.Walk(base, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			if filepath.Ext(path) != ".md" {
				return nil
			}
			res.Scanned++
			f, err := os.Open(path)
			if err != nil {
				res.Failed++
				return nil
			}
			defer f.Close()
			data, err := io.ReadAll(f)
			if err != nil {
				res.Failed++
				return nil
			}
			// parse markdown
			relativePath, err := filepath.Rel(resourcesDir, path)
			if err != nil {
				res.Failed++
				return nil
			}
			doc, err := ParseMarkdown(relativePath, data)
			if err != nil {
				res.Failed++
				return nil
			}
			// compute sha256
			sum := sha256.Sum256(data)
			sha := hex.EncodeToString(sum[:])
			doc.ContentSHA256 = sha
			previousHash, exists, err := s.RecipeDocumentHash(ctx, doc.SourcePath)
			if err != nil {
				res.Failed++
				return nil
			}
			if exists && previousHash == sha {
				return nil
			}

			// upsert doc into recipe_documents
			id, err := s.UpsertRecipeDocument(ctx,
				doc.SourcePath, doc.Kind, doc.Category, doc.Title, doc.Summary,
				doc.Ingredients, doc.DietaryTerms, doc.Difficulty, doc.CaloriesText,
				doc.ContentMarkdown, doc.ContentSHA256, sourceRevision, doc.IsActive,
			)
			if err != nil {
				res.Failed++
				fmt.Printf("[recipe sync] upsert failed %s: %v\n", path, err)
				return nil
			}
			if exists {
				res.Updated++
			} else {
				res.Created++
			}

			// simple chunking: one parent and one child containing full content
			chunk := store.RecipeChildChunk{
				ParentID:      0,
				ChildIndex:    0,
				HeadingPath:   "",
				Content:       doc.ContentMarkdown,
				EmbeddingText: doc.ContentMarkdown,
				ContentHash:   sha,
				TokenCount:    0,
			}
			if err := s.InsertRecipeChildChunks(ctx, id, 1, []store.RecipeChildChunk{chunk}); err != nil {
				res.Failed++
				fmt.Printf("[recipe sync] insert chunks failed %s: %v\n", path, err)
				return nil
			}
			// enqueue index job for background indexing (avoid blocking on embedding)
			if err := s.InsertRecipeIndexJob(ctx, id, 1); err != nil {
				res.Failed++
				fmt.Printf("[recipe sync] enqueue index job failed %s: %v\n", path, err)
				return nil
			}
			fmt.Printf("[recipe sync] upserted %s -> id=%d\n", path, id)

			return nil
		})
	}

	if _, err := os.Stat(dishesDir); err == nil {
		if err := processDir(dishesDir); err != nil {
			return res, err
		}
	}
	if _, err := os.Stat(tipsDir); err == nil {
		if err := processDir(tipsDir); err != nil {
			return res, err
		}
	}

	elapsed := time.Since(start)
	fmt.Printf("[recipe sync] completed in %s: scanned=%d created=%d updated=%d failed=%d\n",
		elapsed.String(), res.Scanned, res.Created, res.Updated, res.Failed)
	if runID != 0 {
		status := "success"
		if res.Failed > 0 {
			status = "failed"
		}
		_ = s.UpdateRecipeSyncRun(ctx, runID, status, res.Scanned, res.Created, res.Updated, res.Deactivated, res.Failed)
	}
	return res, nil
}
