package main

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestShouldLaunchDashboard_DefaultsToFalse(t *testing.T) {
	cmd := &cobra.Command{Use: "start"}
	cmd.Flags().Bool("dashboard", false, "Open the dashboard after the lifecycle command completes")

	if shouldLaunchDashboard(cmd) {
		t.Fatal("expected dashboard launch to be opt-in")
	}
}

func TestShouldLaunchDashboard_TrueWhenFlagSet(t *testing.T) {
	cmd := &cobra.Command{Use: "restart"}
	cmd.Flags().Bool("dashboard", false, "Open the dashboard after the lifecycle command completes")
	if err := cmd.Flags().Set("dashboard", "true"); err != nil {
		t.Fatalf("set dashboard flag: %v", err)
	}

	if !shouldLaunchDashboard(cmd) {
		t.Fatal("expected dashboard launch when --dashboard is set")
	}
}
