package mib

import "testing"

func TestHealthTrapsRequestMedium(t *testing.T) {
	for _, code := range []string{"pethMainPowerUsageOnNotification", "pethMainPowerUsageOffNotification", "agentSwitchCpuRisingThresholdTrap", "agentSwitchCpuFallingThresholdTrap", "agentSwitchCpuFreeMemBelowThresholdTrap", "agentSwitchMbufRisingThresholdTrap", "agentSwitchMbufFallingThresholdTrap"} {
		if !TrapNeedsMedium(code) {
			t.Errorf("%s must request medium", code)
		}
	}
	if TrapNeedsMedium("authenticationFailure") {
		t.Fatal("unrelated trap requests medium")
	}
}
