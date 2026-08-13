package main

import (
	"flag"
	"fmt"
	"os"

	"diary-listener/backend/internal/ragregression"
)

func main() {
	dataset := flag.String("dataset", "testdata/rag/regression/public_v1.jsonl", "regression JSONL")
	manifest := flag.String("manifest", "testdata/rag/regression/public_v1_manifest.json", "manifest JSON")
	flag.Parse()
	if err := ragregression.Validate(*dataset, *manifest); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("deterministic RAG regression fixture valid")
}
