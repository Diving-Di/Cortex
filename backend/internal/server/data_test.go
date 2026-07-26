package server

import (
    "archive/zip"
    "bytes"
    "crypto/sha256"
    "encoding/hex"
    "encoding/json"
    "testing"

    "diary-listener/backend/internal/store"
)

func TestValidateAttachment(t *testing.T) {
    if _, mimeType, err := validateAttachment("note.md", []byte("中文")); err != nil || mimeType != "text/markdown" {
        t.Fatalf("valid markdown rejected: %v", err)
    }
    if _, _, err := validateAttachment("fake.png", []byte("not-png")); err == nil {
        t.Fatal("forged PNG accepted")
    }
    if _, _, err := validateAttachment("empty.txt", nil); err == nil {
        t.Fatal("empty file accepted")
    }
}

func TestSafeArchiveName(t *testing.T) {
    valid := []string{"data.json", "attachments/1/note.md"}
    invalid := []string{"../escape", "/absolute", `dir\file`, "a/../b"}
    for _, name := range valid {
        if !safeArchiveName(name) {
            t.Fatalf("safe name rejected: %s", name)
        }
    }
    for _, name := range invalid {
        if safeArchiveName(name) {
            t.Fatalf("unsafe name accepted: %s", name)
        }
    }
}

func TestValidateBackupManifest(t *testing.T) {
    payload, _ := json.Marshal(store.BackupData{Version: 1})
    digest := sha256.Sum256(payload)
    manifest, _ := json.Marshal(map[string]string{"data.json": hex.EncodeToString(digest[:])})
    var output bytes.Buffer
    archive := zip.NewWriter(&output)
    dataWriter, _ := archive.Create("data.json")
    _, _ = dataWriter.Write(payload)
    manifestWriter, _ := archive.Create("manifest.json")
    _, _ = manifestWriter.Write(manifest)
    _ = archive.Close()

    data, _, err := validateBackup(output.Bytes(), 1<<20)
    if err != nil || data.Version != 1 {
        t.Fatalf("valid backup rejected: %v", err)
    }
}
