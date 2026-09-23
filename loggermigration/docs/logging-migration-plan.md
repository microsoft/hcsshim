# Logging Migration Plan

## Goals

- Decouple from logrus
- Support pluggable logging backends
- Preserve compatibility
- Keep migration incremental

## Phase 1
- Introduce abstraction layer
- Add noop logger
- Add context propagation
- Add logrus adapter

## Phase 2
- Migrate wrapper packages
- Remove direct concrete coupling

## Phase 3
- Add slog/zap adapters
- Deprecate direct logrus exposure

## Non-goals
- Mass rewrites
- Log format changes
- Breaking public APIs
