package knowledge

type BlockKind string

const (
	BlockHeading   BlockKind = "heading"
	BlockParagraph BlockKind = "paragraph"
	BlockList      BlockKind = "list"
	BlockTable     BlockKind = "table"
	BlockCode      BlockKind = "code"
	BlockQuote     BlockKind = "quote"
)

type Block struct {
	Kind        BlockKind
	Text        string
	HeadingPath []string
	PageFrom    int
	PageTo      int
	Order       int
}

type Document struct {
	Title      string
	PageCount  int
	Language   string
	Blocks     []Block
	Characters int
}

type ParentChunk struct {
	Index       int
	Content     string
	HeadingPath []string
	PageFrom    int
	PageTo      int
	TokenCount  int
	Children    []ChildChunk
}

type ChildChunk struct {
	Index         int
	Content       string
	EmbeddingText string
	HeadingPath   []string
	PageFrom      int
	PageTo        int
	TokenCount    int
}

type ExtractLimits struct {
	MaxPages      int
	MaxCharacters int
	TimeoutSecs   int
}
