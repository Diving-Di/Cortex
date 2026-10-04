import { Col, Row, Typography } from 'antd';
import './ContentDiff.css';

// Linear comparison keeps long documents responsive; it does not attempt fuzzy merging.
export default function ContentDiff({ before, after }: { before: string; after: string }) {
  const oldLines = before.split('\n');
  const newLines = after.split('\n');
  let prefix = 0;
  while (
    prefix < oldLines.length &&
    prefix < newLines.length &&
    oldLines[prefix] === newLines[prefix]
  )
    prefix++;
  let suffix = 0;
  while (
    suffix < oldLines.length - prefix &&
    suffix < newLines.length - prefix &&
    oldLines[oldLines.length - 1 - suffix] === newLines[newLines.length - 1 - suffix]
  )
    suffix++;
  return (
    <Row gutter={16} aria-label="内容差异">
      <Col xs={24} md={12}>
        <Typography.Text strong>原文</Typography.Text>
        <pre className="content-diff">
          {oldLines.map((line, i) => (
            <div
              key={i}
              className={i < prefix || i >= oldLines.length - suffix ? '' : 'diff-removed'}
            >
              {line || ' '}
            </div>
          ))}
        </pre>
      </Col>
      <Col xs={24} md={12}>
        <Typography.Text strong>草稿 / 所选版本</Typography.Text>
        <pre className="content-diff">
          {newLines.map((line, i) => (
            <div
              key={i}
              className={i < prefix || i >= newLines.length - suffix ? '' : 'diff-added'}
            >
              {line || ' '}
            </div>
          ))}
        </pre>
      </Col>
    </Row>
  );
}
