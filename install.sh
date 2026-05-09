#!/usr/bin/env bash
# Job Scout — automated installer for macOS and Linux
# Usage: bash install.sh

set -euo pipefail

CYAN='\033[0;36m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; RED='\033[0;31m'; NC='\033[0m'
step()  { echo -e "\n${CYAN}>> $*${NC}"; }
ok()    { echo -e "   ${GREEN}OK: $*${NC}"; }
warn()  { echo -e "   ${YELLOW}!! $*${NC}"; }
fail()  { echo -e "   ${RED}FAIL: $*${NC}"; exit 1; }

echo -e "\n${CYAN}========================================${NC}"
echo -e "${CYAN}  Job Scout — Installer${NC}"
echo -e "${CYAN}========================================${NC}"

OS="$(uname -s)"

# ── 1. Go ─────────────────────────────────────────────────────────────────────
step "Installing Go..."
if command -v go &>/dev/null; then
    ok "$(go version)"
else
    if [[ "$OS" == "Darwin" ]]; then
        command -v brew &>/dev/null || /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
        brew install go
    elif [[ "$OS" == "Linux" ]]; then
        GO_VERSION="1.22.5"
        ARCH="$(uname -m)"
        [[ "$ARCH" == "x86_64" ]] && ARCH="amd64"
        [[ "$ARCH" == "aarch64" ]] && ARCH="arm64"
        curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${ARCH}.tar.gz" -o /tmp/go.tar.gz
        sudo rm -rf /usr/local/go
        sudo tar -C /usr/local -xzf /tmp/go.tar.gz
        rm /tmp/go.tar.gz
        export PATH="$PATH:/usr/local/go/bin"
        echo 'export PATH="$PATH:/usr/local/go/bin"' >> "$HOME/.bashrc"
    fi
    ok "$(go version)"
fi

# ── 2. Node.js ────────────────────────────────────────────────────────────────
step "Installing Node.js..."
if command -v node &>/dev/null; then
    ok "Node.js $(node --version)"
else
    if [[ "$OS" == "Darwin" ]]; then
        brew install node
    elif [[ "$OS" == "Linux" ]]; then
        curl -fsSL https://deb.nodesource.com/setup_lts.x | sudo -E bash -
        sudo apt-get install -y nodejs
    fi
    ok "Node.js $(node --version)"
fi

# ── 3. Ollama ─────────────────────────────────────────────────────────────────
step "Installing Ollama..."
if command -v ollama &>/dev/null; then
    ok "$(ollama --version)"
else
    if [[ "$OS" == "Darwin" ]]; then
        brew install --cask ollama
    elif [[ "$OS" == "Linux" ]]; then
        curl -fsSL https://ollama.com/install.sh | sh
    fi
    ok "$(ollama --version)"
fi

# ── 4. Pull LLM model ─────────────────────────────────────────────────────────
step "Pulling llama3.1:8b model (~5 GB, only on first run)..."
if ollama list 2>/dev/null | grep -q "llama3.1:8b"; then
    ok "llama3.1:8b already installed"
else
    # Ollama needs to be running to pull
    ollama serve &>/dev/null &
    OLLAMA_PID=$!
    sleep 3
    ollama pull llama3.1:8b
    kill $OLLAMA_PID 2>/dev/null || true
    ok "llama3.1:8b downloaded"
fi

# ── 5. Frontend dependencies ──────────────────────────────────────────────────
step "Installing frontend dependencies..."
cd "$(dirname "$0")/client"
npm install --silent
cd - > /dev/null
ok "npm packages installed"

# ── 6. Build frontend ─────────────────────────────────────────────────────────
step "Building frontend..."
cd "$(dirname "$0")/client"
npm run build --silent
cd - > /dev/null
ok "Frontend built"

# ── 7. .env file ──────────────────────────────────────────────────────────────
step "Setting up .env..."
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
if [[ -f "$SCRIPT_DIR/.env" ]]; then
    ok ".env already exists"
else
    cp "$SCRIPT_DIR/.env.example" "$SCRIPT_DIR/.env"
    warn ".env created — open it and fill in your Adzuna API keys"
fi

# ── 8. Go dependencies ────────────────────────────────────────────────────────
step "Downloading Go dependencies..."
cd "$(dirname "$0")/server"
go mod download
cd - > /dev/null
ok "Go modules ready"

# ── 9. profile.md ─────────────────────────────────────────────────────────────
step "Checking profile.md..."
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
if [[ -f "$SCRIPT_DIR/profile.md" ]]; then
    ok "profile.md found"
else
    warn "No profile.md found — copy your CV/profile to profile.md in the project root"
fi

echo -e "\n${GREEN}========================================${NC}"
echo -e "${GREEN}  Setup complete!${NC}"
echo -e "${GREEN}  Run: bash start.sh${NC}"
echo -e "${GREEN}========================================${NC}\n"
