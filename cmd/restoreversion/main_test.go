package main

import "testing"

func TestRunRequiresVersion(t *testing.T) {
	if err := run("", "restored", false); err == nil {
		t.Fatal("expected missing version error")
	}
}

func TestRunRejectsWhitespaceVersion(t *testing.T) {
	if err := run("v0.1.13 beta", "restored", false); err == nil {
		t.Fatal("expected invalid version error")
	}
}

func TestVersionFromBuildInfoDoesNotReturnDevel(t *testing.T) {
	if got := versionFromBuildInfo(); got == "(devel)" {
		t.Fatal("versionFromBuildInfo returned (devel)")
	}
}
