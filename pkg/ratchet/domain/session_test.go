package domain

import (
	"testing"
)

func TestSessionHasToolBeenCalled(t *testing.T) {
	session := NewSession(SessionID("test"))

	if session.HasToolBeenCalled("List Tags") {
		t.Error("Expected false for uncalled tool")
	}

	session.RecordToolCall("List Tags")
	if !session.HasToolBeenCalled("List Tags") {
		t.Error("Expected true for called tool")
	}
}

func TestSessionAddRemoveToken(t *testing.T) {
	session := NewSession(SessionID("test"))
	token := TokenValue("test-token")

	session.AddToken("Create Cell", token)
	if !session.HasValidToken("Create Cell", token) {
		t.Error("Expected token to be present")
	}

	session.RemoveToken("Create Cell", token)
	if session.HasValidToken("Create Cell", token) {
		t.Error("Expected token to be removed")
	}
}
