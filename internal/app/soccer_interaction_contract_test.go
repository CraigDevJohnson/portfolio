package app

import (
	"strings"
	"testing"
)

func TestSoccerJavaScriptOwnsExactTargetBusyAndIntentAwareFocus(t *testing.T) {
	js := readTask2Artifact(t, "cmd", "web", "static", "js", "main.js")
	soccerGateStart := strings.Index(js, "function bindSoccerResponseGate")
	soccerGateEnd := strings.Index(js[soccerGateStart:], "function announceSoccerTargetUpdate")
	if soccerGateStart < 0 || soccerGateEnd < 0 {
		t.Fatal("Soccer JavaScript lacks a bounded response gate")
	}
	soccerGate := js[soccerGateStart : soccerGateStart+soccerGateEnd]
	for _, marker := range []string{"xhr.addEventListener('loadend', cleanup", "soccerProcessingContext?.xhr === xhr", "soccerProcessingContext = null"} {
		if !strings.Contains(soccerGate, marker) {
			t.Errorf("Soccer response gate loadend cleanup lacks %q", marker)
		}
	}
	for _, marker := range []string{
		"beginSoccerRequest(evt)",
		"settleSoccerRequest(evt)",
		"const soccerTarget = getSoccerRequestTarget",
		"soccerTarget.setAttribute('aria-busy', 'true')",
		"soccerTarget.removeAttribute('aria-busy')",
		"soccerRequestKeyboardActivations.has",
		"announceSoccerTargetUpdate",
		"'htmx:sendAbort'",
		"'htmx:oobAfterSwap'",
		"'htmx:oobBeforeSwap'",
		"invalidateDisplacedSoccerTarget",
		"activeSoccerTargets.delete(targetID)",
		"latestSoccerRequestIDs.delete(targetID)",
		"soccerProcessingContext",
		"processing.id < latestTargetRequestID",
		"clearSoccerProcessingContext(evt)",
	} {
		if !strings.Contains(js, marker) {
			t.Errorf("Soccer JavaScript lacks interaction contract marker %q", marker)
		}
	}
	if strings.Contains(js, "if (evt.detail.target.id === 'games-container') {\n      focusSwappedRegion(evt.detail.target)") {
		t.Error("Soccer swaps still focus the results region regardless of pointer intent")
	}
}

func TestSoccerJavaScriptInitializesServerOpenModalAndPreservesLockedActions(t *testing.T) {
	js := readTask2Artifact(t, "cmd", "web", "static", "js", "main.js")
	for _, marker := range []string{
		"initializeServerOpenSoccerModal",
		"setSoccerModalVisibility(true)",
		"#soccer-import-jwt",
		"action.hasAttribute('data-selection-locked')",
	} {
		if !strings.Contains(js, marker) {
			t.Errorf("Soccer JavaScript lacks modal/locked-action marker %q", marker)
		}
	}
}

func TestSoccerJavaScriptPersistsSelectionsByTeamFingerprint(t *testing.T) {
	js := readTask2Artifact(t, "cmd", "web", "static", "js", "main.js")
	for _, marker := range []string{
		"portfolio:soccer:selection:",
		"data-team-fingerprint",
		"window.sessionStorage",
		"version: 1",
		"upcoming:",
		"past:",
		"persistSoccerSelection",
		"restoreSoccerSelection",
		"clearSoccerSelection",
		"soccer-workflow-reset",
		"soccer-logout",
	} {
		if !strings.Contains(js, marker) {
			t.Errorf("Soccer JavaScript lacks selection persistence marker %q", marker)
		}
	}
}

func TestSoccerJavaScriptShowsAndRestoresActionLoadingLabels(t *testing.T) {
	js := readTask2Artifact(t, "cmd", "web", "static", "js", "main.js")
	for _, marker := range []string{
		"data-soccer-feedback",
		"control.dataset.loadingText",
		"loadingOriginalText",
		"buttonText.textContent = control.dataset.loadingText.replace",
		"buttonText.textContent = control.dataset.loadingOriginalText",
	} {
		if !strings.Contains(js, marker) {
			t.Errorf("Soccer JavaScript lacks local loading-label marker %q", marker)
		}
	}
}
