#!/bin/bash
#
# skoob-agent setup script for macOS
# This script installs a LaunchAgent plist file and configures SSH to use skoob-agent
#
# Usage:
#   ./setup_darwin.sh         # Dry run - shows what would be done
#   ./setup_darwin.sh install # Actually performs installation
#

set -e

# Configuration
PLIST_NAME="com.user.skoob-agent.plist"
LAUNCH_AGENTS_DIR="$HOME/Library/LaunchAgents"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SKOOB_AGENT_SOURCE="$SCRIPT_DIR/skoob-agent"
SKOOB_AGENT_PATH="/usr/local/bin/skoob-agent"
SSH_CONFIG="$HOME/.ssh/config"
SSH_SOCKET_PATH="~/.ssh/skoob-agent.sock"

# Check if this is a dry run or actual install
DRY_RUN=true
if [ "$1" = "install" ]; then
    DRY_RUN=false
fi

check_platform() {
    if [[ "$OSTYPE" != "darwin"* ]]; then
        echo "Error: This installer only works on macOS (Darwin)"
        echo "Current OS: $OSTYPE"
        exit 1
    fi
}

validate_binary() {
    if [ ! -f "$SKOOB_AGENT_SOURCE" ]; then
        echo "Error: skoob-agent binary not found at $SKOOB_AGENT_SOURCE"
        echo "Please build the binary first:"
        echo "  Run: ./build.sh"
        exit 1
    fi
    
    if [ ! -x "$SKOOB_AGENT_SOURCE" ]; then
        echo "Error: skoob-agent binary is not executable at $SKOOB_AGENT_SOURCE"
        exit 1
    fi
}

setup_ssh_config() {
    if [ "$DRY_RUN" = true ]; then
        echo "[DRY RUN] Would configure SSH to use skoob-agent..."
        echo "[DRY RUN] Would create ~/.ssh/ directory if needed"
        
        # Check if SSH config already has skoob-agent configuration
        if [ -f "$SSH_CONFIG" ] && grep -q "skoob-agent" "$SSH_CONFIG"; then
            echo "[DRY RUN] SSH config already contains skoob-agent configuration"
            return 0
        fi
        
        if [ -f "$SSH_CONFIG" ]; then
            echo "[DRY RUN] Would create backup of existing SSH config"
        fi
        
        echo "[DRY RUN] Would add to $SSH_CONFIG:"
        echo "[DRY RUN]   # Added by skoob-agent setup $(date +%Y-%m-%d)"
        echo "[DRY RUN]   Host *"
        echo "[DRY RUN]       IdentityAgent \"$SSH_SOCKET_PATH\""
        return 0
    fi
    
    echo "Configuring SSH to use skoob-agent..."
    
    mkdir -p "$HOME/.ssh"
    
    # Check if SSH config already has skoob-agent configuration
    if [ -f "$SSH_CONFIG" ] && grep -q "skoob-agent" "$SSH_CONFIG"; then
        echo "SSH config already contains skoob-agent configuration"
        return 0
    fi
    
    # Create backup of existing config if it exists
    if [ -f "$SSH_CONFIG" ]; then
        cp "$SSH_CONFIG" "$SSH_CONFIG.backup.$(date +%Y%m%d_%H%M%S)"
        echo "Created backup of existing SSH config"
    fi
    
    # Add skoob-agent configuration
    {
        echo ""
        echo "# Added by skoob-agent setup $(date +%Y-%m-%d)"
        echo "Host *"
        echo "    IdentityAgent \"$SSH_SOCKET_PATH\""
    } >> "$SSH_CONFIG"
    
    echo "✓ SSH config updated to use skoob-agent"
}

install_binary() {
    if [ "$DRY_RUN" = true ]; then
        echo "[DRY RUN] Would copy binary to /usr/local/bin/"
        echo "[DRY RUN]   Source: $SKOOB_AGENT_SOURCE"
        echo "[DRY RUN]   Destination: $SKOOB_AGENT_PATH"
        echo "[DRY RUN] Would create /usr/local/bin/ directory if needed"
        return 0
    fi
    
    echo "Installing skoob-agent binary to /usr/local/bin/..."
    
    # Create /usr/local/bin if it doesn't exist
    sudo mkdir -p /usr/local/bin
    
    # Copy the binary
    sudo cp "$SKOOB_AGENT_SOURCE" "$SKOOB_AGENT_PATH"
    
    # Make sure it's executable
    sudo chmod +x "$SKOOB_AGENT_PATH"
    
    echo "✓ Binary installed to $SKOOB_AGENT_PATH"
}

unload_service() {
    if [ "$DRY_RUN" = true ]; then
        echo "[DRY RUN] Would check if skoob-agent service is running"
        if launchctl list | grep -q "com.user.skoob-agent" 2>/dev/null; then
            echo "[DRY RUN] Would unload running skoob-agent service"
        else
            echo "[DRY RUN] No running skoob-agent service found"
        fi
        return 0
    fi
    
    echo "Checking for running skoob-agent service..."
    if launchctl list | grep -q "com.user.skoob-agent" 2>/dev/null; then
        echo "Unloading running skoob-agent service..."
        launchctl unload "$LAUNCH_AGENTS_DIR/$PLIST_NAME" 2>/dev/null || true
        echo "✓ Service unloaded"
    else
        echo "No running skoob-agent service found"
    fi
}

load_service() {
    if [ "$DRY_RUN" = true ]; then
        echo "[DRY RUN] Would load skoob-agent service after installation"
        echo "[DRY RUN] Would run: launchctl load \"$LAUNCH_AGENTS_DIR/$PLIST_NAME\""
        return 0
    fi
    
    echo "Loading skoob-agent service..."
    launchctl load "$LAUNCH_AGENTS_DIR/$PLIST_NAME"
    
    # Give it a moment to start and then check status
    sleep 2
    if launchctl list | grep -q "com.user.skoob-agent" 2>/dev/null; then
        echo "✓ Service loaded and running"
    else
        echo "⚠ Service loaded but may not be running properly"
        echo "  Check logs: tail -f /tmp/skoob-agent.{out,err}"
    fi
}

install_plist() {
    if [ "$DRY_RUN" = true ]; then
        echo "[DRY RUN] Would install skoob-agent LaunchAgent for macOS..."
        echo "[DRY RUN] Would create directory: $LAUNCH_AGENTS_DIR"
        if [ -f "$LAUNCH_AGENTS_DIR/$PLIST_NAME" ]; then
            echo "[DRY RUN] Would replace existing plist: $LAUNCH_AGENTS_DIR/$PLIST_NAME"
        else
            echo "[DRY RUN] Would create new plist: $LAUNCH_AGENTS_DIR/$PLIST_NAME"
        fi
        echo "[DRY RUN] Plist would reference:"
        echo "[DRY RUN]   Binary path: $SKOOB_AGENT_PATH"
        echo "[DRY RUN]   Socket path: $HOME/.ssh/skoob-agent.sock"
        return 0
    fi
    
    echo "Installing skoob-agent LaunchAgent for macOS..."
    
    mkdir -p "$LAUNCH_AGENTS_DIR"
    
    # Create or replace plist file
    if [ -f "$LAUNCH_AGENTS_DIR/$PLIST_NAME" ]; then
        echo "Replacing existing $PLIST_NAME..."
    else
        echo "Creating $PLIST_NAME..."
    fi
    
    cat > "$LAUNCH_AGENTS_DIR/$PLIST_NAME" << EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
  <dict>
    <key>Label</key>
    <string>com.user.skoob-agent</string>

    <key>ProgramArguments</key>
    <array>
      <string>$SKOOB_AGENT_PATH</string>
      <string>-l</string>
      <string>$HOME/.ssh/skoob-agent.sock</string>
    </array>

    <key>RunAtLoad</key>
    <true/>

    <key>KeepAlive</key>
    <true/>

    <key>StandardErrorPath</key>
    <string>/tmp/skoob-agent.err</string>

    <key>StandardOutPath</key>
    <string>/tmp/skoob-agent.out</string>
  </dict>
</plist>
EOF

}

show_yubikey_info() {
    echo "Checking YubiKey X.509 certificate in slot 9a..."
    echo ""
    
    # Try to display the X.509 certificate details using ykman
    if command -v ykman >/dev/null 2>&1; then
        echo "X.509 Certificate Details:"
        echo "=========================="
        
        if ykman piv certificates export 9a - 2>/dev/null | openssl x509 -text -noout 2>/dev/null; then
            echo ""
            echo "✓ Successfully displayed X.509 certificate from slot 9a"
            echo "  The private key corresponding to this certificate is secured in the YubiKey"
            echo "  and is used for SSH authentication via skoob-agent"
        else
            echo "⚠ Could not retrieve X.509 certificate from slot 9a"
            echo "  YubiKey may not be connected or slot 9a may not be configured"
        fi
    else
        echo "⚠ ykman not found - cannot display X.509 certificate details"
        echo "  Install YubiKey Manager: brew install ykman"
    fi
    
    echo ""
    echo "SSH Public Key (derived from X.509 certificate):"
    echo "================================================"
    
    if [ "$DRY_RUN" = false ]; then
        # Try to get SSH public key using skoob-agent
        if command -v "$SKOOB_AGENT_PATH" >/dev/null 2>&1; then
            if SSH_AUTH_SOCK="$HOME/.ssh/skoob-agent.sock" ssh-add -L 2>/dev/null | head -1; then
                SSH_AUTH_SOCK="$HOME/.ssh/skoob-agent.sock" ssh-add -L 2>/dev/null | head -1
                echo "✓ SSH public key retrieved via skoob-agent"
            fi
        fi
    fi
    
    # Try alternative methods that work in both modes
    if SSH_AUTH_SOCK="$SSH_AUTH_SOCK" ssh-add -L 2>/dev/null | grep -q "YubiKey\|PIV"; then
        echo "Via existing SSH agent:"
        SSH_AUTH_SOCK="$SSH_AUTH_SOCK" ssh-add -L 2>/dev/null | grep "YubiKey\|PIV" | head -1
    elif command -v ykman >/dev/null 2>&1; then
        # Convert X.509 certificate to SSH public key format
        ssh_pubkey=$(ykman piv certificates export 9a - 2>/dev/null | openssl x509 -pubkey -noout 2>/dev/null | ssh-keygen -i -m PKCS8 -f /dev/stdin 2>/dev/null)
        if [ -n "$ssh_pubkey" ]; then
            echo "$ssh_pubkey YubiKey_PIV_Slot_9a"
            echo "✓ SSH public key derived from X.509 certificate"
        else
            echo "⚠ Could not convert X.509 certificate to SSH public key format"
        fi
    fi
    
    if [ "$DRY_RUN" = true ]; then
        echo ""
        echo "[DRY RUN] In actual installation, would also try skoob-agent socket"
    fi
    
    echo ""
    echo "Alternative ways to view the SSH public key:"
    echo "  ssh-add -L  (when skoob-agent is running)"
    echo "  ykman piv certificates export 9a - | openssl x509 -pubkey -noout | ssh-keygen -i -m PKCS8 -f /dev/stdin"
    
    # Check if OpenSC is available for alternative method
    if [ -f "/usr/local/lib/opensc-pkcs11.so" ]; then
        echo "  ssh-keygen -D /usr/local/lib/opensc-pkcs11.so"
    else
        echo "  ssh-keygen -D /usr/local/lib/opensc-pkcs11.so  (requires OpenSC: brew install opensc)"
    fi
}

show_usage_instructions() {
    echo ""
    if [ "$DRY_RUN" = true ]; then
        echo "✓ Dry run complete!"
        echo ""
        echo "To actually perform the installation, run:"
        echo "  ./setup_darwin.sh install"
        echo ""
        echo "This would:"
        echo "  - Stop running skoob-agent service (if running)"
        echo "  - Copy binary to: $SKOOB_AGENT_PATH"
        echo "  - Install LaunchAgent to: $LAUNCH_AGENTS_DIR/$PLIST_NAME"
        echo "  - Update SSH config: $SSH_CONFIG"
        echo "  - Start skoob-agent service"
        echo "  - Enable automatic startup on login"
        return 0
    fi
    
    echo "✓ Setup complete!"
    echo ""
    echo "Binary installed to: $SKOOB_AGENT_PATH"
    echo "LaunchAgent installed to: $LAUNCH_AGENTS_DIR/$PLIST_NAME"
    echo "SSH config updated: $SSH_CONFIG"
    echo ""
    echo "Service management commands:"
    echo "  Unload the service:  launchctl unload \"$LAUNCH_AGENTS_DIR/$PLIST_NAME\""
    echo "  Load the service:    launchctl load \"$LAUNCH_AGENTS_DIR/$PLIST_NAME\""
    echo "  Check if running:    launchctl list | grep skoob-agent"
    echo "  View logs:           tail -f /tmp/skoob-agent.{out,err}"
    echo ""
    echo "The agent will automatically start on login and restart if it crashes."
    echo "SSH will now use skoob-agent for all connections."
}

main() {
    check_platform
    validate_binary
    unload_service
    setup_ssh_config
    install_binary
    install_plist
    load_service
    show_yubikey_info
    show_usage_instructions
}

main "$@"