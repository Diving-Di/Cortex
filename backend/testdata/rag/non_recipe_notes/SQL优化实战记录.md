# SQL 优化实战记录

## 背景

某次线上查询监控发现，知识库搜索接口的 P95 延迟从 200ms 跳升到 850ms。排查后发现问题出在 `SearchKnowledge` 的三路召回 CTE 上。

## 问题定位

### 原始查询分析

使用 `EXPLAIN ANALYZE` 查看执行计划：

```sql
EXPLAIN ANALYZE
WITH eligible AS (
  SELECT c.id, c.parent_id, c.document_id, c.index_version,
         c.embedding_text, c.embedding, d.title, d.source_type,
         d.note_id, d.stored_path
  FROM knowledge_child_chunks c
  JOIN knowledge_documents d ON d.tenant_id = c.tenant_id
    AND d.id = c.document_id
  LEFT JOIN notes n ON n.tenant_id = d.tenant_id
    AND n.id = d.note_id
  WHERE c.tenant_id = 'abc123'
    AND d.status = 'ready'
    AND d.deleted_at IS NULL
    AND d.knowledge_enabled
    AND c.index_version = d.active_index_version
    AND c.embedding IS NOT NULL
    AND (d.source_type <> 'note' OR n.deleted_at IS NULL)
)
SELECT count(*) FROM eligible;
```

### 发现的问题

| 问题 | 具体表现 | 影响程度 |
|------|----------|----------|
| HNSW 索引扫描范围过大 | pgvector 选择了过大的 ef_search | 向量召回耗时 +120ms |
| FTS GIN 索引膨胀 | search_vector GIN 索引 2.3GB | 索引扫描 +80ms |
| 嵌套 RRF 重复扫描 | eligible CTE 被各路由 CTE 多次执行 | 总冗余扫描 ~3x |
| JOIN 过滤条件未下推 | JOIN 后在内存中过滤删除的笔记 | 额外传入行数 ~15% |

### 表大小统计

```sql
SELECT
    tablename,
    pg_size_pretty(pg_total_relation_size(schemaname||'.'||tablename)) AS size
FROM pg_tables
WHERE schemaname = 'public'
  AND tablename LIKE 'knowledge_%'
ORDER BY pg_total_relation_size(schemaname||'.'||tablename) DESC;
```

输出：

```
knowledge_child_chunks  | 3.8 GB
knowledge_parent_chunks | 1.2 GB
knowledge_documents     | 48 MB
knowledge_index_jobs    | 12 MB
knowledge_message_sources | 8 MB
```

## 优化方案

### 1. 向量索引调优

```sql
-- 调整 HNSW 参数
CREATE INDEX CONCURRENTLY ix_knowledge_child_vector_v2
  ON knowledge_child_chunks
  USING hnsw (embedding vector_cosine_ops)
  WITH (m = 16, ef_construction = 200)
  WHERE embedding IS NOT NULL;

-- 后来的索引使用新参数重建
DROP INDEX ix_knowledge_child_vector;
ALTER INDEX ix_knowledge_child_vector_v2 RENAME TO ix_knowledge_child_vector;
```

关键参数说明：
- `m = 16`：每个节点的最大连接数，较大值提高召回率但增加构建时间和内存
- `ef_construction = 200`：构建时的搜索宽度，较大值构建更好的图但更慢

### 2. 查询改写——MATERIALIZED CTE

```sql
WITH eligible AS MATERIALIZED (
  -- 同上，但加了 MATERIALIZED 提示 PostgreSQL 物化这个 CTE
  -- 避免被后续 CTE 重复执行
),
v AS (...),
f AS (...),
...
```

实测效果：eligible 的重复扫描从 3 次降为 1 次，总耗时减少约 30%。

### 3. 部分索引缩小范围

```sql
-- 为活跃 chunk 建部分索引
CREATE INDEX ix_knowledge_child_active_vector
  ON knowledge_child_chunks USING hnsw (embedding vector_cosine_ops)
  WHERE embedding IS NOT NULL
    AND index_version > 0;
```

### 优化效果汇总

| 指标 | 优化前 | 优化后 | 变化 |
|------|--------|--------|------|
| 检索 P50 | 137ms | 98ms | **-28%** |
| 检索 P95 | 281ms | 175ms | **-38%** |
| 检索 P99 | 450ms | 240ms | **-47%** |
| 索引大小（child chunks） | 3.8GB | 2.5GB | **-34%** |

## 经验总结

1. **从监控开始**：没有 P95 延迟监控，这个问题可能会在更大的数据量下暴露
2. **EXPLAIN ANALYZE 是好朋友**：不要靠猜，执行计划会告诉你真相
3. **MATERIALIZED CTE 是坑也是宝**：默认情况下 PostgreSQL 可能展开 CTE，导致重复扫描
4. **部分索引是性价比最高的优化**：只索引活跃数据，空间和写入成本同时降低
5. **pgvector 的默认参数保守**：对于 50 万+ 向量的场景，需要调优 m 和 ef_search
