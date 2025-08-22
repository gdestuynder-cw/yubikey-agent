# skoob-agent

<div align="center">
  <img src="skoob.png" alt="skoob-agent" width="120">
</div>

**skoob-agent** is a seamless SSH agent for YubiKeys, forked from [yubikey-agent](https://github.com/FiloSottile/yubikey-agent) and customized for CoreWeave.

## What's Different

This fork includes the following changes from the original yubikey-agent:

- **Enhanced dialog prompts** - PIN and touch dialogs show build information and custom branding
- **Embedded icon** - Custom skoob.png icon embedded in binary for consistent branding
- **Improved setup script** - macOS setup script with dry-run capability and certificate display
- **Build versioning** - Version and build information embedded in binaries and certificates
- **Manages SSH config** - Configures ~/.ssh/config to use skoob-agent by default for all hosts.

## Quick Start

### macOS Installation

1. **Build the binary:**
   ```bash
   ./build.sh
   ```

2. **Install (with dry-run first):**
   ```bash
   # See what would be installed
   ./setup_darwin.sh
   
   # Actually install
   ./setup_darwin.sh install
   ```

3. **Setup your YubiKey:**
   ```bash
   skoob-agent --setup
   ```


### Manual Commands

```bash
# Generate a new SSH key on YubiKey
skoob-agent --setup

# Reset YubiKey (DESTRUCTIVE)
skoob-agent --really-delete-all-piv-keys

# Check if service is running  
launchctl list | grep skoob-agent

# View logs
tail -f /tmp/skoob-agent.{out,err}

# Get your SSH public key
SSH_AUTH_SOCK=~/.ssh/skoob-agent.sock ssh-add -L
```



## Compatibility

Tested configurations:

| Operating System | YubiKey Model | Firmware Version | Status | Tester | 
|------------------|---------------|------------------|--------| -- |
| macOS 15.6.1 (Sequoia) | YubiKey 5C NFC | 5.2.7 | ✅ Working | komalley |


*Note: Other combinations may work but haven't been tested. Please report your results!*

## Public Key Extraction

Multiple ways to get your SSH public key:

```bash
# Via SSH agent (requires setting SSH_AUTH_SOCK)
SSH_AUTH_SOCK=~/.ssh/skoob-agent.sock ssh-add -L

# Direct from YubiKey using ykman
ykman piv certificates export 9a - | openssl x509 -pubkey -noout | ssh-keygen -i -m PKCS8 -f /dev/stdin

# Using OpenSC (if installed)
ssh-keygen -D /usr/local/lib/opensc-pkcs11.so
```

## Key Management

### Viewing Certificate Details
```bash
# Show X.509 certificate in slot 9a
ykman piv certificates export 9a - | openssl x509 -text -noout

# List all PIV certificates
ykman piv info
```

### Resetting/Deleting Keys
```bash
# Nuclear option: reset entire PIV applet
ykman piv reset
# OR
skoob-agent --really-delete-all-piv-keys

# Surgical: delete specific certificate only  
ykman piv certificates delete 9a
```

## Service Management (Darwin)

```bash
# Unload service
launchctl unload ~/Library/LaunchAgents/com.user.skoob-agent.plist

# Load service
launchctl load ~/Library/LaunchAgents/com.user.skoob-agent.plist

# Check status
launchctl list | grep skoob-agent

# View real-time logs
tail -f /tmp/skoob-agent.out /tmp/skoob-agent.err
```

## Troubleshooting

### Common Issues

1. **Touch prompts not appearing**: This is expected when running as a LaunchAgent. The YubiKey will still work - just touch it when it blinks.

2. **"No YubiKey detected"**: Ensure YubiKey is plugged in and try unplugging/replugging.

3. **PIN locked**: Use `ykman piv access unblock-pin` with your PUK (same as PIN by default).

4. **Need to start fresh**: Use `skoob-agent --really-delete-all-piv-keys` to completely reset.

### Conflicts with Other Software

skoob-agent takes a persistent lock on the YubiKey PIV applet. To release it:
```bash
ssh-add -D
```

This may be needed if you want to use:
- GPG/PGP functionality
- YubiKey Manager for configuration changes  
- Other PIV applications

## Technical Notes

- Uses PIV authentication slot 9a with ECDSA P-256 keys
- Generates random management key stored in PIN-protected metadata
- PIN policy: once per session
- Touch policy: always (every signature operation)
- Socket location: `~/.ssh/skoob-agent.sock`

## Original Project

Based on [yubikey-agent](https://github.com/FiloSottile/yubikey-agent) by Filippo Valsorda.