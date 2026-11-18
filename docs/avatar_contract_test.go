package docs

import (
	"encoding/json"
	"os"
	"sort"
	"testing"
)

// UpdateUserAvatar answers 400 for an avatar that does not exist or is inactive
// and never answers 404 or 422; the published contract must say the same.
var updateUserAvatarStatuses = []string{"200", "400", "401", "403", "429", "500"}

type contractSpec struct {
	Paths map[string]map[string]struct {
		Responses map[string]json.RawMessage `json:"responses"`
	} `json:"paths"`
}

func documentedStatuses(t *testing.T, source string, raw []byte) []string {
	t.Helper()

	var spec contractSpec
	if err := json.Unmarshal(raw, &spec); err != nil {
		t.Fatalf("%s: invalid JSON: %v", source, err)
	}
	operation, ok := spec.Paths["/citizen/{cpf}/avatar"]["put"]
	if !ok {
		t.Fatalf("%s: PUT /citizen/{cpf}/avatar is not documented", source)
	}

	statuses := make([]string, 0, len(operation.Responses))
	for status := range operation.Responses {
		statuses = append(statuses, status)
	}
	sort.Strings(statuses)
	return statuses
}

func assertUpdateUserAvatarStatuses(t *testing.T, source string, raw []byte) {
	t.Helper()

	got := documentedStatuses(t, source, raw)
	if len(got) != len(updateUserAvatarStatuses) {
		t.Fatalf("%s: PUT /citizen/{cpf}/avatar documents %v, want %v", source, got, updateUserAvatarStatuses)
	}
	for i := range got {
		if got[i] != updateUserAvatarStatuses[i] {
			t.Fatalf("%s: PUT /citizen/{cpf}/avatar documents %v, want %v", source, got, updateUserAvatarStatuses)
		}
	}
}

func TestUpdateUserAvatarContract_EmbeddedSwagger(t *testing.T) {
	assertUpdateUserAvatarStatuses(t, "docs.go", []byte(SwaggerInfo.ReadDoc()))
}

func TestUpdateUserAvatarContract_PublishedFiles(t *testing.T) {
	for _, file := range []string{"swagger.json", "openapi-v3.json"} {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		assertUpdateUserAvatarStatuses(t, file, raw)
	}
}
