#!/usr/bin/env bash
# Job Scout — start all services (macOS / Linux)
# Usage: bash start.sh

set -euo pipefail

CYAN='\033[0;36m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; NC='\033[0m'
step() { echo -e "\n${CYAN}>> $*${NC}"; }

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

echo -e "\n${CYAN}========================================${NC}"
echo -e "${CYAN}  Job Scout — Starting${NC}"
echo -e "${CYAN}========================================${NC}"

# ── 1. Ollama ─────────────────────────────────────────────────────────────────
step "Starting Ollama..."
if pgrep -x ollama &>/dev/null; then
    echo -e "   ${GREEN}Ollama already running${NC}"
else
    OLLAMA_NUM_PARALLEL=2 ollama serve &>/dev/null &
    sleep 2
    echo -e "   ${GREEN}Ollama started (2 parallel slots)${NC}"
fi

# ── 2. Frontend build check ───────────────────────────────────────────────────
step "Checking frontend build..."
if [[ ! -f "$SCRIPT_DIR/client/dist/index.html" ]]; then
    echo -e "   ${YELLOW}Building frontend...${NC}"
    cd "$SCRIPT_DIR/client" && npm run build --silent && cd - > /dev/null
fi
echo -e "   ${GREEN}Frontend ready${NC}"

# ── 3. Go server ──────────────────────────────────────────────────────────────
step "Starting Job Scout server..."
echo -e "   ${GREEN}Open http://localhost:8080 in your browser${NC}"
echo -e "   ${YELLOW}Press Ctrl+C to stop${NC}\n"

cd "$SCRIPT_DIR/server"
go run ./cmd/server
