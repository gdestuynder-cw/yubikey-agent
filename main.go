// Copyright 2020 Google LLC
//
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file or at
// https://developers.google.com/open-source/licenses/bsd

package main

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/go-piv/piv-go/v2/piv"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/term"
)

//go:embed skoob.png
var skoobIcon []byte

// Build information - can be set via ldflags
var (
	version   = "unknown"
	buildTime = "unknown"
	buildHash = "unknown"
)

func getVersion() string {
	// Use version set at build time via ldflags
	if version != "unknown" {
		return "v" + version
	}
	
	// Fallback: try to load version from VERSION file (for development)
	if file, err := os.Open("VERSION"); err == nil {
		defer file.Close()
		if versionBytes, err := io.ReadAll(file); err == nil {
			return "v" + strings.TrimSpace(string(versionBytes))
		}
	}
	return "unknown"
}

func getBuildInfo() string {
	version := getVersion()
	result := version
	
	// Add build time if available
	if buildTime != "unknown" {
		result += " (built " + buildTime + ")"
	}
	
	// Add git hash if available
	if buildHash != "unknown" {
		result += " [" + buildHash + "]"
	}
	
	// Try to get additional build info from Go
	if buildInfo, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range buildInfo.Settings {
			if setting.Key == "vcs.revision" && buildHash == "unknown" {
				if len(setting.Value) > 7 {
					result += " [" + setting.Value[:7] + "]"
				} else {
					result += " [" + setting.Value + "]"
				}
				break
			}
		}
	}
	
	return result
}

func getBuildHashForTitle() string {
	// Return just the build hash in brackets for dialog titles
	if buildHash != "unknown" {
		return "[" + buildHash + "]"
	}
	
	// Try to get from Go build info if not set via ldflags
	if buildInfo, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range buildInfo.Settings {
			if setting.Key == "vcs.revision" {
				if len(setting.Value) > 7 {
					return "[" + setting.Value[:7] + "]"
				} else {
					return "[" + setting.Value + "]"
				}
			}
		}
	}
	
	return ""
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of skoob-agent:\n")
		fmt.Fprintf(os.Stderr, "\n")
		fmt.Fprintf(os.Stderr, "\tskoob-agent -setup\n")
		fmt.Fprintf(os.Stderr, "\n")
		fmt.Fprintf(os.Stderr, "\t\tGenerate a new SSH key on the attached YubiKey.\n")
		fmt.Fprintf(os.Stderr, "\n")
		fmt.Fprintf(os.Stderr, "\tskoob-agent -l PATH\n")
		fmt.Fprintf(os.Stderr, "\n")
		fmt.Fprintf(os.Stderr, "\t\tRun the agent, listening on the UNIX socket at PATH.\n")
		fmt.Fprintf(os.Stderr, "\n")
	}

	socketPath := flag.String("l", "", "agent: path of the UNIX socket to listen on")
	resetFlag := flag.Bool("really-delete-all-piv-keys", false, "setup: reset the PIV applet")
	setupFlag := flag.Bool("setup", false, "setup: configure a new YubiKey")
	flag.Parse()

	if flag.NArg() > 0 {
		flag.Usage()
		os.Exit(1)
	}

	if *setupFlag {
		log.SetFlags(0)
		yk := connectForSetup()
		if *resetFlag {
			runReset(yk)
		}
		runSetup(yk)
	} else {
		if *socketPath == "" {
			flag.Usage()
			os.Exit(1)
		}
		runAgent(*socketPath)
	}
}

func runAgent(socketPath string) {
	log.Printf("Starting skoob-agent %s", getBuildInfo())
	
	if term.IsTerminal(int(os.Stdin.Fd())) {
		log.Println("Warning: skoob-agent is meant to run as a background daemon.")
		log.Println("Running multiple instances is likely to lead to conflicts.")
		log.Println("Consider using the launchd or systemd services.")
	}

	a := &Agent{}

	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGHUP)
	go func() {
		for range c {
			a.Close()
		}
	}()

	os.Remove(socketPath)
	if err := os.MkdirAll(filepath.Dir(socketPath), 0777); err != nil {
		log.Fatalln("Failed to create UNIX socket folder:", err)
	}
	l, err := net.Listen("unix", socketPath)
	if err != nil {
		log.Fatalln("Failed to listen on UNIX socket:", err)
	}

	for {
		c, err := l.Accept()
		if err != nil {
			type temporary interface {
				Temporary() bool
			}
			if err, ok := err.(temporary); ok && err.Temporary() {
				log.Println("Temporary Accept error, sleeping 1s:", err)
				time.Sleep(1 * time.Second)
				continue
			}
			log.Fatalln("Failed to accept connections:", err)
		}
		go a.serveConn(c)
	}
}

type Agent struct {
	mu     sync.Mutex
	yk     *piv.YubiKey
	serial uint32
}

var _ agent.ExtendedAgent = &Agent{}

func (a *Agent) serveConn(c net.Conn) {
	if err := agent.ServeAgent(a, c); err != io.EOF {
		log.Println("Agent client connection ended with error:", err)
	}
}

func healthy(yk *piv.YubiKey) bool {
	// We can't use Serial because it locks the session on older firmwares, and
	// can't use Retries because it fails when the session is unlocked.
	_, err := yk.AttestationCertificate()
	return err == nil
}

func (a *Agent) ensureYK() error {
	if a.yk == nil || !healthy(a.yk) {
		if a.yk != nil {
			log.Println("Reconnecting to the YubiKey...")
			a.yk.Close()
		} else {
			log.Println("Connecting to the YubiKey...")
		}
		yk, err := a.connectToYK()
		if err != nil {
			return err
		}
		a.yk = yk
	}
	return nil
}

func (a *Agent) maybeReleaseYK() {
	// On macOS, YubiKey 5s persist the PIN cache even across sessions (and even
	// processes), so we can release the lock on the key, to let other
	// applications like age-plugin-yubikey use it.
	if runtime.GOOS != "darwin" || a.yk.Version().Major < 5 {
		return
	}
	if err := a.yk.Close(); err != nil {
		log.Println("Failed to automatically release YubiKey lock:", err)
	}
	a.yk = nil
}

func (a *Agent) connectToYK() (*piv.YubiKey, error) {
	yk, err := openYK()
	if err != nil {
		return nil, err
	}
	// Cache the serial number locally because requesting it on older firmwares
	// requires switching application, which drops the PIN cache.
	a.serial, _ = yk.Serial()
	return yk, nil
}

func openYK() (yk *piv.YubiKey, err error) {
	cards, err := piv.Cards()
	if err != nil {
		return nil, err
	}
	if len(cards) == 0 {
		return nil, errors.New("no YubiKey detected")
	}
	// TODO: support multiple YubiKeys. For now, select the first one that opens
	// successfully, to skip any internal unused smart card readers.
	for _, card := range cards {
		yk, err = piv.Open(card)
		if err == nil {
			return
		}
	}
	return
}

func (a *Agent) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.yk != nil {
		log.Println("Received HUP, dropping YubiKey transaction...")
		err := a.yk.Close()
		a.yk = nil
		return err
	}
	return nil
}

func getEmbeddedIconPath() (string, func()) {
	// Create a secure temporary file for embedded icon
	if len(skoobIcon) == 0 {
		return "", func() {}
	}
	
	// Create temp file with restrictive permissions
	tmpFile, err := os.CreateTemp("", "skoob-icon-*.png")
	if err != nil {
		log.Printf("DEBUG: Failed to create temp file for icon: %v", err)
		return "", func() {}
	}
	
	// Set restrictive permissions (owner read-only)
	if err := tmpFile.Chmod(0400); err != nil {
		log.Printf("DEBUG: Failed to set temp file permissions: %v", err)
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return "", func() {}
	}
	
	// Write embedded icon data to temp file
	if _, err := tmpFile.Write(skoobIcon); err != nil {
		log.Printf("DEBUG: Failed to write icon data: %v", err)
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return "", func() {}
	}
	
	tmpFile.Close()
	
	// Return cleanup function
	cleanup := func() {
		os.Remove(tmpFile.Name())
	}
	
	return tmpFile.Name(), cleanup
}

func (a *Agent) getPIN() (string, error) {
	r, _ := a.yk.Retries()
	// Use embedded icon with secure temp file
	imagePath, cleanup := getEmbeddedIconPath()
	defer cleanup()

	return getPIN(a.serial, r, imagePath)
}

func (a *Agent) List() ([]*agent.Key, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.ensureYK(); err != nil {
		return nil, fmt.Errorf("could not reach YubiKey: %w", err)
	}
	defer a.maybeReleaseYK()

	pk, err := getPublicKey(a.yk, piv.SlotAuthentication)
	if err != nil {
		return nil, err
	}
	return []*agent.Key{{
		Format:  pk.Type(),
		Blob:    pk.Marshal(),
		Comment: fmt.Sprintf("YubiKey #%d PIV Slot 9a", a.serial),
	}}, nil
}

func getPublicKey(yk *piv.YubiKey, slot piv.Slot) (ssh.PublicKey, error) {
	cert, err := yk.Certificate(slot)
	if err != nil {
		return nil, fmt.Errorf("could not get public key: %w", err)
	}
	switch cert.PublicKey.(type) {
	case *ecdsa.PublicKey:
	case ed25519.PublicKey:
	case *rsa.PublicKey:
	default:
		return nil, fmt.Errorf("unexpected public key type: %T", cert.PublicKey)
	}
	pk, err := ssh.NewPublicKey(cert.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to process public key: %w", err)
	}
	return pk, nil
}

func (a *Agent) Signers() ([]ssh.Signer, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.ensureYK(); err != nil {
		return nil, fmt.Errorf("could not reach YubiKey: %w", err)
	}
	defer a.maybeReleaseYK()

	return a.signers()
}

func (a *Agent) signers() ([]ssh.Signer, error) {
	pk, err := getPublicKey(a.yk, piv.SlotAuthentication)
	if err != nil {
		return nil, err
	}
	priv, err := a.yk.PrivateKey(
		piv.SlotAuthentication,
		pk.(ssh.CryptoPublicKey).CryptoPublicKey(),
		piv.KeyAuth{PINPrompt: a.getPIN},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare private key: %w", err)
	}
	s, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare signer: %w", err)
	}
	return []ssh.Signer{s}, nil
}

func (a *Agent) Sign(key ssh.PublicKey, data []byte) (*ssh.Signature, error) {
	return a.SignWithFlags(key, data, 0)
}

func (a *Agent) SignWithFlags(key ssh.PublicKey, data []byte, flags agent.SignatureFlags) (*ssh.Signature, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.ensureYK(); err != nil {
		return nil, fmt.Errorf("could not reach YubiKey: %w", err)
	}
	defer a.maybeReleaseYK()

	signers, err := a.signers()
	if err != nil {
		return nil, err
	}
	for _, s := range signers {
		if !bytes.Equal(s.PublicKey().Marshal(), key.Marshal()) {
			continue
		}

		// Show touch prompt and wait for user response before proceeding
		// Use embedded icon with secure temp file
		imagePath, cleanup := getEmbeddedIconPath()

		// Show touch prompt in background so it doesn't block signing operation
		log.Printf("DEBUG: About to show touch prompt for serial %d", a.serial)
		go func() {
			defer cleanup()
			log.Printf("DEBUG: Inside touch prompt goroutine")
			if err := promptTouch(a.serial, imagePath); err != nil {
				log.Printf("DEBUG: Touch prompt failed: %v", err)
				// If touch prompt fails, fall back to notification
				showNotification("Waiting for Key touch...")
			} else {
				log.Printf("DEBUG: Touch prompt succeeded")
			}
		}()
		
		// Small delay to ensure notification has time to appear
		log.Printf("DEBUG: Sleeping 100ms for notification display")
		time.Sleep(100 * time.Millisecond)
		log.Printf("DEBUG: About to start signing operation")

		alg := key.Type()
		switch {
		case alg == ssh.KeyAlgoRSA && flags&agent.SignatureFlagRsaSha256 != 0:
			alg = ssh.SigAlgoRSASHA2256
		case alg == ssh.KeyAlgoRSA && flags&agent.SignatureFlagRsaSha512 != 0:
			alg = ssh.SigAlgoRSASHA2512
		}
		// TODO: maybe retry if the PIN is not correct?
		return s.(ssh.AlgorithmSigner).SignWithAlgorithm(rand.Reader, data, alg)
	}
	return nil, fmt.Errorf("no private keys match the requested public key")
}

func showNotification(message string) {
	switch runtime.GOOS {
	case "darwin":
		message = strings.ReplaceAll(message, `\`, `\\`)
		message = strings.ReplaceAll(message, `"`, `\"`)
		appleScript := `display notification "%s" with title "skoob-agent"`
		exec.Command("osascript", "-e", fmt.Sprintf(appleScript, message)).Run()
	case "linux":
		exec.Command("notify-send", "-i", "dialog-password", "skoob-agent", message).Run()
	}
}

func (a *Agent) Extension(extensionType string, contents []byte) ([]byte, error) {
	return nil, agent.ErrExtensionUnsupported
}

var ErrOperationUnsupported = errors.New("operation unsupported")

func (a *Agent) Add(key agent.AddedKey) error {
	return ErrOperationUnsupported
}
func (a *Agent) Remove(key ssh.PublicKey) error {
	return ErrOperationUnsupported
}
func (a *Agent) RemoveAll() error {
	return a.Close()
}
func (a *Agent) Lock(passphrase []byte) error {
	return ErrOperationUnsupported
}
func (a *Agent) Unlock(passphrase []byte) error {
	return ErrOperationUnsupported
}
