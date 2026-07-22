# Changelog

本文件记录 Diary Listener 项目的重要变更。

格式参考 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本号遵循[语义化版本](https://semver.org/lang/zh-CN/)。

## [Unreleased]

### Git 提交记录

> 维护规则：每次功能、修复、重构或文档提交完成后，在此记录其短哈希、日期和提交说明，再提交 Changelog。由于提交哈希由提交内容计算得出，Changelog 维护提交不记录自身，避免产生无限递归提交。

| Commit | 日期 | 类型 | 说明 |
|---|---|---|---|
| `1ee9378` | 2026-07-22 | docs | `docs: add Diary Listener refactor SDD` |

### Added

- 新增 `development-standards/` 项目开发规范目录。
- 新增 Diary Listener 重构软件设计说明书（SDD），用于定义项目目标、系统架构、数据设计、接口、安全要求、实施计划及验收标准。

### Changed

- 将根目录下的 `SDD.md` 归档到 `development-standards/SDD.md`，统一管理项目设计与开发规范文档。
