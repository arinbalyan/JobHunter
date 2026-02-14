# Contributing to JobHunter

Thanks for your interest in contributing. This document covers the process for
reporting issues, suggesting features, and submitting code changes.

---

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Reporting Issues](#reporting-issues)
- [Suggesting Features](#suggesting-features)
- [Development Setup](#development-setup)
- [Making Changes](#making-changes)
- [Pull Request Process](#pull-request-process)
- [Code Standards](#code-standards)
- [Testing](#testing)

---

## Code of Conduct

This project follows the [Contributor Covenant Code of Conduct](CODE_OF_CONDUCT.md).
By participating, you agree to uphold this standard.

---

## Reporting Issues

Before opening a new issue:

1. Search [existing issues](https://github.com/arinbalyan/JobHunter/issues) to
   avoid duplicates
2. If you find a related issue, add your information as a comment

When creating a new issue, include:

- A clear, descriptive title
- Steps to reproduce the problem
- Expected vs actual behavior
- Python version and operating system
- Relevant log output or error messages (redact any personal information)

---

## Suggesting Features

Feature requests are welcome. Open an issue with:

- A description of the feature and the problem it solves
- Any alternative approaches you have considered
- Whether you are willing to implement it yourself

---

## Development Setup

1. Fork the repository and clone your fork:

   ```bash
   git clone https://github.com/your-username/JobHunter.git
   cd JobHunter
   ```

2. Install dependencies with [uv](https://docs.astral.sh/uv/):

   ```bash
   uv sync
   ```

3. Copy the environment template:

   ```bash
   cp .env.example .env
   ```

4. Run the test suite to verify your setup:

   ```bash
   uv run pytest
   ```

---

## Making Changes

1. Create a branch from `main`:

   ```bash
   git checkout -b feature/your-feature-name
   ```

2. Make your changes in small, focused commits

3. Write or update tests for any new or changed functionality

4. Run the full test suite and linter before pushing:

   ```bash
   uv run pytest
   uv run ruff check src/ tests/
   ```

5. Push your branch and open a pull request

---

## Pull Request Process

1. Fill out the pull request template with a description of your changes
2. Link any related issues
3. Ensure all CI checks pass (tests, linting)
4. Request a review from a maintainer
5. Address any review feedback
6. Once approved, a maintainer will merge your pull request

Keep pull requests focused on a single concern. If you have multiple unrelated
changes, submit them as separate pull requests.

---

## Code Standards

- **Python 3.10+** -- use modern syntax and type hints where appropriate
- **Ruff** for linting and formatting (line length: 88)
- **Functional patterns** -- prefer pure functions and composition over deep
  class hierarchies
- **Environment-driven configuration** -- all settings come from environment
  variables, never hardcoded
- **Minimal comments** -- write clear code that documents itself; add comments
  only when the "why" is not obvious
- **No secrets in code** -- never commit credentials, API keys, or personal
  data

---

## Testing

All changes should include tests. The project uses pytest:

```bash
# Run all tests
uv run pytest

# Run with verbose output
uv run pytest -v

# Run a specific test file
uv run pytest tests/unit/test_storage.py

# Run with coverage report
uv run pytest --cov=jobspy_v2
```

Tests are organized by layer:

| File | Covers |
|------|--------|
| `test_config.py` | Settings loading and validation |
| `test_storage.py` | Storage backends (Sheets, CSV) |
| `test_core.py` | Scraper, email generation, sending, dedup, reporter |
| `test_workflows.py` | Pipeline orchestration and CLI |
| `test_utils.py` | Utility functions |
| `test_scheduler.py` | Scheduling logic |

Aim for meaningful test coverage. Every new feature or bug fix should have at
least one corresponding test.
