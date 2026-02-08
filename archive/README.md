# Archived Implementations

This directory contains legacy and experimental implementations preserved for reference.

## Contents

### python-legacy/
Original Python implementation (Docker-based, 8-stage pipeline).
Last active: January 2025

### rust-experimental/
Exploratory Rust implementation (incomplete).
Last active: January 2024

### python-tests/
Unit tests for Python implementation.

### go-original/
Reference showing original Go location before reorganization.

## Why Archived?

The Go implementation (now in root) became primary due to:
- Better performance (single binary)
- More comprehensive features
- Active development and testing
- Simpler deployment

## Restoration

To explore archived code:
```bash
git log --all --full-history -- archive/python-legacy/
git checkout <commit> -- archive/python-legacy/
```
