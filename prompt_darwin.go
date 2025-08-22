// Copyright 2020 Google LLC
//
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file or at
// https://developers.google.com/open-source/licenses/bsd

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
)

func getPIN(serial uint32, retries int, imagePath string) (string, error) {
	// NOTE: Using hardcoded JavaScript instead of Go templates because the template
	// system was causing "Can't convert types" errors that were a pain in the ass to debug
	// Add icon if available
	iconOption := ""
	if imagePath != "" {
		iconOption = fmt.Sprintf(`,
	withIcon: Path("%s")`, imagePath)
	}

	// Get build hash for title - need to access from main package
	titleSuffix := getBuildHashForTitle()
	
	scriptContent := fmt.Sprintf(`
var app = Application.currentApplication()
app.includeStandardAdditions = true
app.displayDialog(
	"Key Serial: %d (%d tries left)\n\nPlease enter your PIN:", {
    defaultAnswer: "",
	withTitle: "CoreWeave skoob-agent %s - PIN required",
    buttons: ["Cancel", "OK"],
    defaultButton: "OK",
	cancelButton: "Cancel",
    hiddenAnswer: true%s
})`, int(serial), int(retries), titleSuffix, iconOption)

	log.Printf("DEBUG: Generated JavaScript:\n%s", scriptContent)

	c := exec.Command("osascript", "-s", "se", "-l", "JavaScript")
	c.Stdin = bytes.NewBufferString(scriptContent)
	out, err := c.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("failed to execute osascript (exit status %d): %s\nGenerated JS:\n%s", exitErr.ExitCode(), string(exitErr.Stderr), scriptContent)
		}
		return "", fmt.Errorf("failed to execute osascript: %v", err)
	}
	var x struct {
		PIN string `json:"textReturned"`
	}
	if err := json.Unmarshal(out, &x); err != nil {
		return "", fmt.Errorf("failed to parse osascript output: %v", err)
	}
	return x.PIN, nil
}

func promptTouch(serial uint32, imagePath string) error {
	// NOTE: Using hardcoded JavaScript instead of Go templates because the template
	// system was causing "Can't convert types" errors that were a pain in the ass to debug
	//
	// Touch dialog should be persistent and visible even from LaunchAgent context
	log.Printf("DEBUG: promptTouch called with serial %d", serial)

	// Use a more persistent dialog that stays until dismissed or timeout
	// Force it to appear in foreground even from background service
	// NOTE: YubiKey touch policy timeout is hardcoded at 15 seconds

	// Add icon if available
	iconOption := ""
	if imagePath != "" {
		iconOption = fmt.Sprintf(`,
		withIcon: Path("%s")`, imagePath)
	}

	titleSuffix := getBuildHashForTitle()
	scriptContent := fmt.Sprintf(`
var app = Application.currentApplication()
app.includeStandardAdditions = true

try {
	var result = app.displayDialog("Please touch your YubiKey now.", {
		withTitle: "CoreWeave skoob-agent %s - Touch required",
		buttons: ["Dismiss"],
		defaultButton: "Dismiss",
		givingUpAfter: 5%s
	})
	result
} catch (e) {
	// Dialog was dismissed or timed out - return success
	{buttonReturned: "timeout"}
}`, titleSuffix, iconOption)

	log.Printf("DEBUG: Touch JavaScript:\n%s", scriptContent)

	log.Printf("DEBUG: Running touch dialog synchronously with longer timeout")
	c := exec.Command("osascript", "-s", "se", "-l", "JavaScript")
	c.Stdin = bytes.NewBufferString(scriptContent)

	// Wait for completion - dialog will show for up to 10 seconds
	out, err := c.Output()
	if err != nil {
		log.Printf("DEBUG: Touch dialog completed with error: %v", err)
		if exitErr, ok := err.(*exec.ExitError); ok {
			log.Printf("DEBUG: osascript stderr: %s", string(exitErr.Stderr))
		}
	} else {
		log.Printf("DEBUG: Touch dialog completed successfully: %s", string(out))
	}

	log.Printf("DEBUG: promptTouch returning nil")
	return nil
}
