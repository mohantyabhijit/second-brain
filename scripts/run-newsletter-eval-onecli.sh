#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BACKEND_DIR="$ROOT_DIR/backend"
ONECLI="${ONECLI:-/Users/abhijitmohanty/.local/bin/onecli}"
PROJECT="${ONECLI_PROJECT:-second-brain}"

if [[ -f "$BACKEND_DIR/.env" ]]; then
  set -a
  # shellcheck disable=SC1091
  . "$BACKEND_DIR/.env"
  set +a
fi

if [[ -z "${DATABASE_URL:-}" ]]; then
  if DATABASE_URL="$(security find-generic-password -a "$USER" -s "second-brain/DATABASE_URL" -w 2>/dev/null)"; then
    export DATABASE_URL
  else
    unset DATABASE_URL
  fi
fi
export ONECLI_GATEWAY=true
export OPENAI_SYNTHESIS_MODEL="${OPENAI_SYNTHESIS_MODEL:-gpt-5.4-mini}"
export OPENAI_CHAT_MODEL="${OPENAI_CHAT_MODEL:-$OPENAI_SYNTHESIS_MODEL}"
export NEWSLETTER_EVAL_ITERATIONS="${NEWSLETTER_EVAL_ITERATIONS:-5}"
export NEWSLETTER_EVAL_JUDGE_MODEL="${NEWSLETTER_EVAL_JUDGE_MODEL:-gpt-4o-mini}"
export NEWSLETTER_EVAL_GENERATOR_MODEL="${NEWSLETTER_EVAL_GENERATOR_MODEL:-$OPENAI_SYNTHESIS_MODEL}"
export NEWSLETTER_EVAL_IMPROVER_MODEL="${NEWSLETTER_EVAL_IMPROVER_MODEL:-$OPENAI_SYNTHESIS_MODEL}"
export NEWSLETTER_EVAL_OUTPUT_DIR="${NEWSLETTER_EVAL_OUTPUT_DIR:-$ROOT_DIR/data/runtime/newsletter-experiments}"
export KNOWLEDGE_RUN_PATH="${KNOWLEDGE_RUN_PATH:-$ROOT_DIR/data/runtime/latest-knowledge-run.json}"

cd "$BACKEND_DIR"
exec "$ONECLI" run --project "$PROJECT" -- go run ./cmd/newsletter-eval "$@"
