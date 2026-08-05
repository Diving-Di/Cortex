package knowledge

import "testing"

func TestChunkSkipsHeadingOnlyParent(t *testing.T) {
	markdown := "# 菜谱\n简介\n\n## 操作\n\n### 处理原料\n切丝并腌制。\n"
	parents := Chunk("菜谱", "upload", markdown)
	if len(parents) != 2 {
		t.Fatalf("Chunk() returned %d parents, want 2: %#v", len(parents), parents)
	}
	for _, parent := range parents {
		if len(parent.Heading) == 2 && parent.Heading[1] == "操作" {
			t.Fatalf("heading-only parent was indexed: %#v", parent)
		}
	}
	if got := parents[1].Heading; len(got) != 3 || got[2] != "处理原料" {
		t.Fatalf("child heading path=%v", got)
	}
}

func TestChunkKeepsHeadingWithBody(t *testing.T) {
	parents := Chunk("菜谱", "upload", "## 操作\n第一步。")
	if len(parents) != 1 || len(parents[0].Children) == 0 {
		t.Fatalf("informative heading was dropped: %#v", parents)
	}
}
